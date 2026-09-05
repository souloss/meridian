package meridian

//go:generate go tool oapi-codegen --config configs/codegen-server-types.yaml contracts/openapi.yaml
//go:generate go tool oapi-codegen --config configs/codegen-server.yaml contracts/openapi.yaml
//go:generate go tool oapi-codegen --config configs/codegen-client.yaml contracts/openapi.yaml
//go:generate go tool oapi-codegen --config configs/codegen-server-spec.yaml contracts/openapi.yaml
