package meridian

//go:generate go tool sqlc generate
//go:generate go tool oapi-codegen --config configs/codegen-server-types.yaml build/contracts/api/common/openapi.yaml
//go:generate go tool oapi-codegen --config configs/codegen-server-spec.yaml contracts/openapi.yaml
//go:generate sh ./scripts/generate-api-servers.sh
//go:generate go run ./internal/codegen/strictstub
//go:generate go run ./internal/codegen/domainadapter
//go:generate go run ./internal/codegen/godoc -dir internal/generated/api
//go:generate go run ./internal/codegen/godoc -dir internal/generated/repository
