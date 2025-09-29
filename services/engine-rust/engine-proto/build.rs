fn main() -> Result<(), Box<dyn std::error::Error>> {
    let proto_file = "../../../proto/engine/v1/engine.proto";
    let proto_dir = "../../../proto";
    
    // Tell cargo to invalidate the built crate whenever the proto file changes
    println!("cargo:rerun-if-changed={}", proto_file);
    
    tonic_build::configure()
        .build_server(true)
        .build_client(true)
        .compile(&[proto_file], &[proto_dir])?;
    
    Ok(())
}