use std::{
  collections::HashMap,
  sync::Arc,
  time::{Duration, Instant},
};

use mdns_sd::{ServiceDaemon, ServiceEvent};
use ncro_config::DiscoveryConfig;
use ncro_health::Prober;
use tokio::sync::{Mutex, watch};

pub struct Discovery {
  cfg:    DiscoveryConfig,
  prober: Prober,
  daemon: ServiceDaemon,
  peers:  Arc<Mutex<HashMap<String, (String, Instant)>>>,
}

impl Discovery {
  pub fn new(cfg: DiscoveryConfig, prober: Prober) -> anyhow::Result<Self> {
    Ok(Self {
      cfg,
      prober,
      daemon: ServiceDaemon::new()?,
      peers: Arc::new(Mutex::new(HashMap::new())),
    })
  }

  pub async fn run(
    self,
    mut stop: watch::Receiver<bool>,
  ) -> anyhow::Result<()> {
    let service = format!(
      "{}.{}.",
      self.cfg.service_name.trim_end_matches('.'),
      self.cfg.domain.trim_end_matches('.')
    );
    let receiver = self.daemon.browse(&service)?;
    let peers = Arc::clone(&self.peers);
    let prober = self.prober.clone();
    let priority = self.cfg.priority;
    let mut cleanup = tokio::time::interval(Duration::from_secs(10));
    let expiration = if self.cfg.discovery_time.0.is_zero() {
      Duration::from_secs(30)
    } else {
      self.cfg.discovery_time.0 * 3
    };

    loop {
      tokio::select! {
          _ = stop.changed() => { let _ = self.daemon.shutdown(); return Ok(()); }
          _ = cleanup.tick() => {
              let stale = {
                  let mut guard = peers.lock().await;
                  let now = Instant::now();
                  let stale = guard.iter().filter(|(_, (_, seen))| now.duration_since(*seen) > expiration).map(|(k, (u, _))| (k.clone(), u.clone())).collect::<Vec<_>>();
                  for (key, _) in &stale { guard.remove(key); }
                  stale
              };
              for (_, url) in stale { tracing::info!(url, "removing stale peer"); prober.remove_upstream(&url).await; }
          }
          event = tokio::task::spawn_blocking({ let receiver = receiver.clone(); move || receiver.recv_timeout(Duration::from_millis(500)).ok() }) => {
              if let Ok(Some(ServiceEvent::ServiceResolved(info))) = event {
                  let Some(addr) = info.get_addresses().iter().next().map(mdns_sd::ScopedIp::to_ip_addr) else { continue; };
                  let url = format!("http://{}", std::net::SocketAddr::new(addr, info.get_port()));
                  let key = info.get_fullname().to_string();
                  let is_new = peers.lock().await.insert(key, (url.clone(), Instant::now())).is_none();
                  if is_new { tracing::info!(url, "discovered nix-serve instance"); prober.add_upstream(url, priority).await; }
              }
          }
      }
    }
  }
}
