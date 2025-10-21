module github.com/cartridge/web

go 1.22

require (
	github.com/go-chi/chi/v5 v5.0.10
	github.com/prometheus/client_golang v1.18.0
	github.com/rs/zerolog v1.31.0
)

replace github.com/go-chi/chi/v5 => ../orchestrator-go/internal/thirdparty/chi

replace github.com/rs/zerolog => ../orchestrator-go/internal/thirdparty/zerolog

replace github.com/prometheus/client_golang => ../orchestrator-go/internal/thirdparty/prometheus
