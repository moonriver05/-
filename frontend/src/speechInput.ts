// Average adjacent samples to produce mono, little-endian PCM16 at 16 kHz.
export class PCMEncoder {
  private sum = 0
  private count = 0
  private phase = 0
  private pending: number[] = []
  private samples = 0
  private sourceRate: number
  constructor(sourceRate: number) {
    this.sourceRate = sourceRate
    if (sourceRate < 16000) throw new Error('浏览器音频采样率过低')
  }
  push(input: Float32Array): Uint8Array[] {
    const frames: Uint8Array[] = []
    for (const value of input) {
      if (this.samples >= 16000 * 60) break
      this.sum += value; this.count++; this.phase += 16000
      if (this.phase >= this.sourceRate) {
        const sample = Math.max(-1, Math.min(1, this.sum / this.count))
        const pcm = Math.round(sample * (sample < 0 ? 32768 : 32767))
        this.pending.push(pcm & 255, (pcm >> 8) & 255)
        this.samples++; this.phase -= this.sourceRate; this.sum = 0; this.count = 0
        if (this.pending.length === 6400) frames.push(this.flush())
      }
    }
    return frames
  }
  flush(): Uint8Array { const frame = Uint8Array.from(this.pending); this.pending = []; return frame }
}

type VoiceEvent = { event: 'ready' | 'transcript' | 'done' | 'error'; text?: string; error?: string }
const workletCode = `
class MathWikiMic extends AudioWorkletProcessor {
  process(inputs) {
    const channels = inputs[0];
    if (channels && channels.length) {
      const mono = new Float32Array(channels[0].length);
      for (const channel of channels) for (let i=0; i<mono.length; i++) mono[i] += channel[i]/channels.length;
      this.port.postMessage(mono, [mono.buffer]);
    }
    return true;
  }
}
registerProcessor('mathwiki-mic', MathWikiMic);
`

export class SpeechInput {
  private stream?: MediaStream
  private context?: AudioContext
  private node?: AudioWorkletNode
  private source?: MediaStreamAudioSourceNode
  private socket?: WebSocket
  private timer?: number
  private encoder?: PCMEncoder
  private active = true
  private recording = false
  private completed = false
  private lastText = ''
  private text: (text: string) => void
  private state: (state: 'recording' | 'transcribing' | 'done') => void
  private error: (message: string) => void
  constructor(text: (text: string) => void, state: (state: 'recording' | 'transcribing' | 'done') => void, error: (message: string) => void) {
    this.text = text; this.state = state; this.error = error
  }
  async start() {
    if (!window.isSecureContext) throw new Error('语音输入需要 HTTPS，请通过 HTTPS 地址访问网站')
    if (!navigator.mediaDevices?.getUserMedia || !window.AudioContext || !window.AudioWorkletNode) throw new Error('当前浏览器不支持语音输入，请使用新版 Chrome 或 Edge')
    this.context = new AudioContext()
    await this.context.resume()
    this.stream = await navigator.mediaDevices.getUserMedia({audio: {channelCount: 1, echoCancellation: true, noiseSuppression: true}})
    if (!this.active) { this.releaseAudio(); return }
    this.encoder = new PCMEncoder(this.context.sampleRate)
    const moduleURL = URL.createObjectURL(new Blob([workletCode], {type:'text/javascript'}))
    try { await this.context.audioWorklet.addModule(moduleURL) } finally { URL.revokeObjectURL(moduleURL) }
    if (!this.active) return
    this.source = this.context.createMediaStreamSource(this.stream)
    this.node = new AudioWorkletNode(this.context, 'mathwiki-mic')
    const socket = this.socket = new WebSocket(`${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}/api/v1/speech/stream`)
    this.timer = window.setTimeout(()=>this.fail('语音服务连接超时，请重试'),15000)
    socket.onmessage = event => {
      if (!this.active) return
      let message: VoiceEvent
      try { message = JSON.parse(String(event.data)) as VoiceEvent } catch { this.fail('语音服务返回了无效数据'); return }
      if (message.event === 'error') { this.fail(message.error || '语音识别失败'); return }
      if (message.event === 'ready') {
        window.clearTimeout(this.timer)
        this.recording = true
        this.state('recording')
        this.node!.port.onmessage = event => {
          if (!this.recording) return
          for (const frame of this.encoder!.push(event.data as Float32Array)) {
            if (socket.bufferedAmount > 128000) { this.fail('网络较慢，请稍后重试语音输入'); return }
            socket.send(frame)
          }
        }
        this.source!.connect(this.node!)
        this.node!.connect(this.context!.destination) // Worklet output is silence.
        this.timer = window.setTimeout(()=>this.stop(),60000)
      }
      if (message.text) { this.lastText = message.text; this.text(message.text) }
      if (message.event === 'done') {
        this.completed = true
        this.cancel()
        if (!this.lastText) this.error('没有识别到语音，请靠近麦克风再试')
        this.state('done')
      }
    }
    socket.onerror = () => this.fail('无法连接语音服务，请检查登录状态、HTTPS 和服务器配置')
    socket.onclose = () => { if (this.active && !this.completed) this.fail('语音连接中断，请重新录音') }
  }
  stop() {
    if (!this.recording || this.socket?.readyState !== WebSocket.OPEN) return
    this.recording = false
    const tail = this.encoder!.flush()
    if (tail.length) this.socket.send(tail)
    this.socket.send('stop')
    this.releaseAudio()
    window.clearTimeout(this.timer)
    this.state('transcribing')
    this.timer = window.setTimeout(()=>this.fail('语音识别超时，请重试'),15000)
  }
  cancel() { this.active = false; this.recording = false; window.clearTimeout(this.timer); this.releaseAudio(); this.socket?.close() }
  private releaseAudio() {
    if (this.node) { this.node.port.onmessage = null; this.node.disconnect() }
    this.source?.disconnect()
    this.stream?.getTracks().forEach(track=>track.stop())
    if (this.context && this.context.state !== 'closed') void this.context.close().catch(()=>{})
  }
  private fail(message: string) { if (!this.active) return; this.cancel(); this.state('done'); this.error(message) }
}
