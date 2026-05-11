mod cli;
mod config;
mod db;
mod discovery;
mod health;
mod mesh;
mod metrics;
mod narinfo;
mod router;
mod server;

#[tokio::main]
async fn main() -> anyhow::Result<()> {
  cli::run().await
}
