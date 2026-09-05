import { defineConfig } from 'orval'

export default defineConfig({
  meridian: {
    input: '../contracts/openapi.yaml',
    output: {
      mode: 'tags-split',
      target: './app/api/generated/client.ts',
      schemas: './app/api/generated/models',
      client: 'vue-query',
      httpClient: 'fetch',
      clean: true,
      mock: {
        path: './app/api/generated/mocks',
        generators: [{ type: 'msw' }]
      },
      override: {
        mutator: {
          path: './app/api/fetcher.ts',
          name: 'meridianFetch'
        },
        paramsSerializer: {
          path: './app/api/query-params.ts',
          name: 'serializeQueryParams'
        }
      }
    }
  }
})
