module github.com/cartridge/orchestrator

go 1.22

replace github.com/go-chi/chi/v5 => ./internal/thirdparty/chi

replace github.com/rs/zerolog => ./internal/thirdparty/zerolog

replace github.com/prometheus/client_golang => ./internal/thirdparty/prometheus

replace github.com/google/uuid => ./internal/thirdparty/uuid

require (
        github.com/go-chi/chi/v5 v5.0.10
        github.com/google/uuid v1.5.0
        github.com/prometheus/client_golang v1.18.0
        github.com/rs/zerolog v1.31.0
)
