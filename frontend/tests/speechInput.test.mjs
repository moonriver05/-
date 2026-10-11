import test from 'node:test'
import assert from 'node:assert/strict'
import { PCMEncoder, SpeechInput } from '../src/speechInput.ts'

test('48 kHz microphone becomes 16 kHz mono PCM16 across fragmented worklet messages', () => {
  const encoder = new PCMEncoder(48000)
  const frames = []
  for (let i=0; i<75; i++) frames.push(...encoder.push(new Float32Array(128).fill(.5)))
  assert.equal(frames.length, 1)
  assert.equal(frames[0].length, 6400)
  assert.equal(new DataView(frames[0].buffer).getInt16(0,true),16384)
  assert.equal(encoder.flush().length,0)
})
test('60 second hard limit and saturation apply before sending audio', () => {
  const encoder = new PCMEncoder(16000)
  const frames = encoder.push(new Float32Array(16000*61).fill(-2))
  assert.equal(frames.reduce((sum,frame)=>sum+frame.length,0),16000*60*2)
  assert.equal(new DataView(frames[0].buffer).getInt16(0,true),-32768)
  assert.equal(encoder.flush().length,0)
  assert.deepEqual(encoder.push(Float32Array.of(1)),[])
})

test('cancelling while microphone permission is pending releases the late stream without cloud usage', async () => {
  let grant, stopped = 0, closed = 0
  const originalWindow = globalThis.window
  const originalNavigator = Object.getOwnPropertyDescriptor(globalThis, 'navigator')
  class Context {
    state = 'running'
    resume() { return Promise.resolve() }
    close() { this.state = 'closed'; closed++; return Promise.resolve() }
  }
  globalThis.window = {isSecureContext:true,AudioContext:Context,AudioWorkletNode:class {},clearTimeout}
  globalThis.AudioContext = Context
  Object.defineProperty(globalThis,'navigator',{configurable:true,value:{mediaDevices:{getUserMedia:()=>new Promise(resolve=>{grant=resolve})}}})
  try {
    const voice = new SpeechInput(()=>assert.fail('unexpected transcript'),()=>{},()=>{})
    const started = voice.start()
    await Promise.resolve()
    voice.cancel()
    grant({getTracks:()=>[{stop:()=>stopped++}]})
    await started
    assert.equal(stopped,1)
    assert.equal(closed,1)
  } finally {
    globalThis.window = originalWindow
    delete globalThis.AudioContext
    if (originalNavigator) Object.defineProperty(globalThis,'navigator',originalNavigator)
    else delete globalThis.navigator
  }
})
