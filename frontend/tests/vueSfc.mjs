import { readFileSync } from 'node:fs'
import { parse, compileScript } from 'vue/compiler-sfc'
import ts from 'typescript'

// Compile real local components for DOM integration tests, stubbing only the
// unrelated panels. This keeps new text-rendering components in the test path.
export function compileSFC(url, components = {}) {
  const { descriptor } = parse(readFileSync(url, 'utf8'))
  const source = compileScript(descriptor, { id: url.pathname, inlineTemplate: true }).content
    .replace(/import (\w+) from ['"][^'"]+\.vue['"]/g, (_, name) => components[name] ? `import ${name} from '${components[name]}'` : `const ${name} = { render: () => null }`)
    .replace(/import ['"][^'"]+\.css['"]/g, '')
    .replace(/from ['"]([^'"]+)['"]/g, (_, specifier) => {
      const resolved = specifier.startsWith('data:') ? specifier : specifier.startsWith('.') ? new URL(specifier + (specifier.endsWith('.ts') ? '' : '.ts'), url).href : import.meta.resolve(specifier)
      return `from ${JSON.stringify(resolved)}`
    })
  const js = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ESNext } }).outputText
  return `data:text/javascript;base64,${Buffer.from(js).toString('base64')}`
}
