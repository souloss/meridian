import fs from 'node:fs'
import path from 'node:path'
import ts from 'typescript'

const checkOnly = process.argv.includes('--check')
const generatedDirectory = new URL('../app/api/generated/', import.meta.url)
const files = generatedFiles(generatedDirectory)
const undocumented = []

for (const filename of files) {
  const source = fs.readFileSync(filename, 'utf8')
  const syntax = ts.createSourceFile(filename, source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS)
  const insertions = []

  for (const statement of syntax.statements) {
    if (!isExported(statement)) continue
    if (ts.isVariableStatement(statement)) {
      for (const declaration of statement.declarationList.declarations) {
        const name = declarationName(declaration.name)
        ensureDocumentation(source, statement, name, declarationComment(name), insertions)
        documentTypeMembers(source, declaration.type, name, insertions)
      }
      continue
    }

    const name = declarationName(statement.name)
    if (!name) continue
    ensureDocumentation(source, statement, name, declarationComment(name), insertions)
    if (ts.isInterfaceDeclaration(statement)) {
      for (const member of statement.members) documentMember(source, member, name, insertions)
    } else if (ts.isTypeAliasDeclaration(statement)) {
      documentTypeMembers(source, statement.type, name, insertions)
    }
  }

  if (insertions.length === 0) continue
  undocumented.push(`${path.relative(generatedDirectory.pathname, filename)}: ${insertions.map((item) => item.name).join(', ')}`)
  if (checkOnly) continue

  let output = source
  for (const insertion of insertions.sort((left, right) => right.position - left.position)) {
    output = output.slice(0, insertion.position) + insertion.text + output.slice(insertion.position)
  }
  fs.writeFileSync(filename, output)
}

if (checkOnly && undocumented.length > 0) {
  console.error(`Generated TypeScript documentation is stale in ${undocumented.length} files:`)
  console.error(undocumented.slice(0, 50).join('\n'))
  process.exit(1)
}

function generatedFiles(directory) {
  return fs.readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const child = new URL(`${entry.name}${entry.isDirectory() ? '/' : ''}`, directory)
    if (entry.isDirectory()) return generatedFiles(child)
    if (entry.name === 'index.ts' || !entry.name.endsWith('.ts')) return []
    return [child.pathname]
  })
}

function isExported(node) {
  return node.modifiers?.some((modifier) => modifier.kind === ts.SyntaxKind.ExportKeyword) ?? false
}

function declarationName(node) {
  if (!node) return ''
  if (ts.isIdentifier(node) || ts.isStringLiteral(node) || ts.isNumericLiteral(node)) return node.text
  return ''
}

function ensureDocumentation(source, node, name, comment, insertions) {
  if (!name || hasDocumentation(source, node)) return
  insertions.push({ ...lineInsertion(source, node.getStart(), `/** ${comment} */`), name })
}

function documentTypeMembers(source, node, owner, insertions) {
  if (!node) return
  if (ts.isTypeLiteralNode(node)) {
    for (const member of node.members) documentMember(source, member, owner, insertions)
    return
  }
  ts.forEachChild(node, (child) => documentTypeMembers(source, child, owner, insertions))
}

function documentMember(source, member, owner, insertions) {
  const name = declarationName(member.name)
  if (name) ensureDocumentation(source, member, name, memberComment(owner, name), insertions)
  if ('type' in member) documentTypeMembers(source, member.type, `${owner}.${name}`, insertions)
}

function hasDocumentation(source, node) {
  if (Array.isArray(node.jsDoc) && node.jsDoc.length > 0) return true
  const leadingText = source.slice(node.getFullStart(), node.getStart())
  return /\/\*\*[\s\S]*\*\/\s*$/.test(leadingText)
}

function lineInsertion(source, position, comment) {
  const lineStart = source.lastIndexOf('\n', position - 1) + 1
  const indentation = source.slice(lineStart, position)
  if (indentation.trim() === '') return { position: lineStart, text: `${indentation}${comment}\n` }
  return { position, text: `${comment} ` }
}

function declarationComment(name) {
  const label = splitIdentifier(name)
  if (name.endsWith('Body')) return `${name} is the request body type for its generated OpenAPI operation.`
  if (/Response(?:[1-5][0-9]{2}|Success|Error)?$/.test(name)) return `${name} represents a declared HTTP response from the ${label} operation.`
  if (name.endsWith('Params')) return `${name} contains parameters accepted by its generated OpenAPI operation.`
  if (name.endsWith('QueryResult')) return `${name} is the resolved data returned by its generated Vue Query hook.`
  if (name.endsWith('QueryError')) return `${name} is the error type returned by its generated Vue Query hook.`
  if (name.startsWith('use')) return `${name} executes its OpenAPI operation through TanStack Vue Query.`
  if (name.startsWith('get') && name.endsWith('Url')) return `${name} builds the relative URL for its OpenAPI operation.`
  if (name.includes('QueryOptions')) return `${name} builds TanStack Query options for its OpenAPI operation.`
  if (name.includes('MutationOptions')) return `${name} builds TanStack Mutation options for its OpenAPI operation.`
  if (name.includes('Mock')) return `${name} provides generated MSW behavior for contract tests.`
  return `${name} is generated from the Meridian OpenAPI contract for ${label}.`
}

function memberComment(owner, name) {
  const comments = {
    data: 'Data contains the decoded response payload.',
    status: 'Status is the HTTP response status code.',
    headers: 'Headers contains the HTTP response headers.',
    body: 'Body contains the serialized request payload.',
  }
  return comments[name] ?? `${capitalize(name)} carries the ${splitIdentifier(name)} value for ${owner}.`
}

function splitIdentifier(value) {
  return value
    .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
    .replace(/([A-Z]+)([A-Z][a-z])/g, '$1 $2')
    .replace(/[._-]+/g, ' ')
    .toLowerCase()
}

function capitalize(value) {
  return value.charAt(0).toUpperCase() + value.slice(1)
}
