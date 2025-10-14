module github.com/cartridge/weights

go 1.22

require (
        github.com/rs/zerolog v1.31.0
        google.golang.org/grpc v1.65.0
        google.golang.org/protobuf v1.34.2
)

replace github.com/rs/zerolog => ../orchestrator-go/internal/thirdparty/zerolog
