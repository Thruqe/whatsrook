/// Removes SQLite WAL and SHM sidecar files for the given database path.
///
/// Deletes `<db_path>-shm` and `<db_path>-wal` if they exist, allowing a
/// clean shutdown without leaving stale journal files on disk. Missing files
/// are silently ignored; other I/O errors are printed to stderr.
///
/// # Arguments
/// * `db_path` – Base path of the SQLite database (without the `-shm`/`-wal` suffix).
pub async fn cleanup_db(db_path: &str) {
    for suffix in ["-shm", "-wal"] {
        let path = format!("{db_path}{suffix}");
        match tokio::fs::remove_file(&path).await {
            Ok(_) => {}
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => {}
            Err(e) => eprintln!("Failed to remove {path}: {e}"),
        }
    }
}

/// Waits for a process termination signal.
///
/// Resolves on whichever arrives first:
/// - **Ctrl-C** (`SIGINT`) on all platforms.
/// - **SIGTERM** on Unix platforms (compiled out on non-Unix targets, where
///   this branch becomes a permanently pending future).
pub async fn shutdown_signal() {
    use tokio::signal;

    let ctrl_c = async {
        signal::ctrl_c().await.expect("failed to listen for Ctrl+C");
    };

    #[cfg(unix)]
    let terminate = async {
        signal::unix::signal(signal::unix::SignalKind::terminate())
            .expect("failed to install SIGTERM handler")
            .recv()
            .await;
    };

    #[cfg(not(unix))]
    let terminate = std::future::pending::<()>();

    tokio::select! {
        _ = ctrl_c => {},
        _ = terminate => {},
    }
}
