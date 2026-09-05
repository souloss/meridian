export function serializeQueryParams(params: object = {}): string {
  const search = new URLSearchParams()

  const append = (key: string, value: unknown): void => {
    if (value === undefined) {
      return
    }
    if (Array.isArray(value)) {
      for (const item of value) {
        append(key, item)
      }
      return
    }
    if (value !== null && typeof value === 'object') {
      for (const [nestedKey, nestedValue] of Object.entries(value)) {
        append(`${key}[${nestedKey}]`, nestedValue)
      }
      return
    }
    search.append(key, value === null ? 'null' : String(value))
  }

  for (const [key, value] of Object.entries(params)) {
    append(key, value)
  }
  return search.toString()
}
