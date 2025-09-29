fn main() -> Result<(), Box<dyn std::error::Error>> {
    // Generate protobuf code for engine and replay services
    tonic_build::configure()
        .build_server(false) // We only need clients
        .compile(
            &[
                "../../proto/engine/v1/engine.proto",
                "../../proto/replay/v1/replay.proto",
            ],
            &["../../proto"],
        )?;
    Ok(())
}