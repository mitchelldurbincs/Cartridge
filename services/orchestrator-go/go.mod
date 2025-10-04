module github.com/cartridge/orchestrator

go 1.22

replace github.com/go-chi/chi/v5 => ./internal/thirdparty/chi

replace github.com/rs/zerolog => ./internal/thirdparty/zerolog

require (
	github.com/go-chi/chi/v5 v5.0.10
	github.com/rs/zerolog v1.31.0
)
