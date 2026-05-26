use std::{env, fs, time::Duration};

use serde::{Deserialize, Deserializer};
use thiserror::Error;
use url::Url;

#[derive(Debug, Error)]
pub enum ConfigError {
  #[error("read config: {0}")]
  Read(#[from] std::io::Error),
  #[error("parse config: {0}")]
  Parse(#[from] toml::de::Error),
  #[error("{0}")]
  Validation(String),
}

#[cfg(test)]
mod tests {
  use super::*;

  #[test]
  fn loads_defaults() -> Result<(), ConfigError> {
    let cfg = Config::load(None)?;
    assert_eq!(cfg.server.listen, ":8080");
    assert_eq!(cfg.cache.max_entries, 100_000);
    assert_eq!(cfg.upstreams.len(), 1);
    cfg.validate()?;
    Ok(())
  }

  #[test]
  fn parses_duration_toml() -> Result<(), toml::de::Error> {
    let cfg: Config = toml::from_str(
      "[server]\ncache_priority = 40\n\n[cache]\nttl = \"2h\"\n",
    )?;
    assert_eq!(cfg.server.cache_priority, 40);
    assert_eq!(cfg.cache.ttl.0, Duration::from_secs(7200));
    Ok(())
  }

  #[test]
  fn validates_mass_query_limits() -> Result<(), toml::de::Error> {
    let cfg: Config = toml::from_str(
      "[cache.mass_query]\nmax_concurrent_races = \
       0\nper_upstream_max_inflight = 1\nin_memory_negative_ttl = \
       \"5s\"\nupstream_cooldown = \"10s\"\n",
    )?;
    let err = cfg.validate().expect_err("expected validation failure");
    assert!(
      err
        .to_string()
        .contains("cache.mass_query.max_concurrent_races must be >= 1")
    );
    Ok(())
  }
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct HumanDuration(pub Duration);

impl Default for HumanDuration {
  fn default() -> Self {
    Self(Duration::ZERO)
  }
}

impl<'de> Deserialize<'de> for HumanDuration {
  fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
  where
    D: Deserializer<'de>,
  {
    humantime_serde::deserialize(deserializer).map(Self)
  }
}

#[derive(Debug, Clone, Default, Deserialize)]
#[serde(default)]
pub struct UpstreamConfig {
  pub url:        String,
  pub priority:   i32,
  pub public_key: String,
}

#[derive(Debug, Clone, Deserialize)]
#[serde(default)]
pub struct ServerConfig {
  pub listen:         String,
  pub cache_priority: i32,
  pub read_timeout:   HumanDuration,
  pub write_timeout:  HumanDuration,
}

impl Default for ServerConfig {
  fn default() -> Self {
    Self {
      listen:         ":8080".to_string(),
      cache_priority: 30,
      read_timeout:   HumanDuration(Duration::from_secs(30)),
      write_timeout:  HumanDuration(Duration::from_secs(30)),
    }
  }
}

#[derive(Debug, Clone, Deserialize)]
#[serde(default)]
pub struct CacheConfig {
  pub db_path:       String,
  pub max_entries:   i64,
  pub ttl:           HumanDuration,
  pub negative_ttl:  HumanDuration,
  pub latency_alpha: f64,
  pub mass_query:    MassQueryConfig,
}

impl Default for CacheConfig {
  fn default() -> Self {
    Self {
      db_path:       "/var/lib/ncro/routes.db".to_string(),
      max_entries:   100_000,
      ttl:           HumanDuration(Duration::from_secs(60 * 60)),
      negative_ttl:  HumanDuration(Duration::from_secs(10 * 60)),
      latency_alpha: 0.3,
      mass_query:    MassQueryConfig::default(),
    }
  }
}

#[derive(Debug, Clone, Deserialize)]
#[serde(default)]
pub struct MassQueryConfig {
  pub max_concurrent_races:      u32,
  pub per_upstream_max_inflight: u32,
  pub in_memory_negative_ttl:    HumanDuration,
  pub upstream_cooldown:         HumanDuration,
}

impl Default for MassQueryConfig {
  fn default() -> Self {
    Self {
      max_concurrent_races:      64,
      per_upstream_max_inflight: 8,
      in_memory_negative_ttl:    HumanDuration(Duration::from_secs(5)),
      upstream_cooldown:         HumanDuration(Duration::from_secs(15)),
    }
  }
}

#[derive(Debug, Clone, Default, Deserialize)]
#[serde(default)]
pub struct PeerConfig {
  pub addr:       String,
  pub public_key: String,
}

#[derive(Debug, Clone, Deserialize)]
#[serde(default)]
pub struct MeshConfig {
  pub enabled:          bool,
  pub bind_addr:        String,
  pub peers:            Vec<PeerConfig>,
  #[serde(rename = "private_key")]
  pub private_key_path: String,
  pub gossip_interval:  HumanDuration,
}

impl Default for MeshConfig {
  fn default() -> Self {
    Self {
      enabled:          false,
      bind_addr:        "0.0.0.0:7946".to_string(),
      peers:            Vec::new(),
      private_key_path: String::new(),
      gossip_interval:  HumanDuration(Duration::from_secs(30)),
    }
  }
}

/// Which address families to use when registering mDNS-discovered peers.
///
/// A discovered service may advertise multiple addresses (IPv4 and IPv6).
/// This option controls which are registered as upstreams.  `any` (default)
/// registers all routable addresses and lets the router race them; set `ipv4`
/// or `ipv6` to restrict to a single family when the upstream server is known
/// to only listen on one (e.g. nix-serve binds to 0.0.0.0 by default).
#[derive(Debug, Clone, Default, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "lowercase")]
pub enum AddressFamily {
  #[default]
  Any,
  Ipv4,
  Ipv6,
}

#[derive(Debug, Clone, Deserialize)]
#[serde(default)]
pub struct DiscoveryConfig {
  pub enabled:        bool,
  pub service_name:   String,
  pub domain:         String,
  pub discovery_time: HumanDuration,
  pub priority:       i32,
  pub address_family: AddressFamily,
}

impl Default for DiscoveryConfig {
  fn default() -> Self {
    Self {
      enabled:        false,
      service_name:   "_nix-serve._tcp".to_string(),
      domain:         "local".to_string(),
      discovery_time: HumanDuration(Duration::from_secs(5)),
      priority:       20,
      address_family: AddressFamily::default(),
    }
  }
}

#[derive(Debug, Clone, Deserialize)]
#[serde(default)]
pub struct LoggingConfig {
  pub level:  String,
  pub format: String,
}

impl Default for LoggingConfig {
  fn default() -> Self {
    Self {
      level:  "info".to_string(),
      format: "json".to_string(),
    }
  }
}

#[derive(Debug, Clone, Deserialize)]
#[serde(default)]
pub struct Config {
  pub server:    ServerConfig,
  pub upstreams: Vec<UpstreamConfig>,
  pub cache:     CacheConfig,
  pub mesh:      MeshConfig,
  pub discovery: DiscoveryConfig,
  pub logging:   LoggingConfig,
}

impl Default for Config {
  fn default() -> Self {
    Self {
      server:    ServerConfig::default(),
      upstreams: vec![UpstreamConfig {
        url:        "https://cache.nixos.org".to_string(),
        priority:   10,
        public_key: String::new(),
      }],
      cache:     CacheConfig::default(),
      mesh:      MeshConfig::default(),
      discovery: DiscoveryConfig::default(),
      logging:   LoggingConfig::default(),
    }
  }
}

impl Config {
  pub fn load(path: Option<&str>) -> Result<Self, ConfigError> {
    let mut cfg = if let Some(path) = path.filter(|p| !p.is_empty()) {
      let data = fs::read_to_string(path)?;
      toml::from_str::<Self>(&data)?
    } else {
      Self::default()
    };

    if let Ok(v) = env::var("NCRO_LISTEN")
      && !v.is_empty()
    {
      cfg.server.listen = v;
    }
    if let Ok(v) = env::var("NCRO_DB_PATH")
      && !v.is_empty()
    {
      cfg.cache.db_path = v;
    }
    if let Ok(v) = env::var("NCRO_LOG_LEVEL")
      && !v.is_empty()
    {
      cfg.logging.level = v;
    }

    Ok(cfg)
  }

  pub fn validate(&self) -> Result<(), ConfigError> {
    if self.upstreams.is_empty() {
      return Err(ConfigError::Validation(
        "at least one upstream is required".to_string(),
      ));
    }
    for (i, upstream) in self.upstreams.iter().enumerate() {
      if upstream.url.is_empty() {
        return Err(ConfigError::Validation(format!(
          "upstream[{i}]: URL is empty"
        )));
      }
      Url::parse(&upstream.url).map_err(|err| {
        ConfigError::Validation(format!(
          "upstream[{i}]: invalid URL {:?}: {err}",
          upstream.url
        ))
      })?;
      if !upstream.public_key.is_empty() && !upstream.public_key.contains(':') {
        return Err(ConfigError::Validation(format!(
          "upstream[{i}]: public_key must be in 'name:base64(key)' Nix format"
        )));
      }
    }
    if self.server.listen.is_empty() {
      return Err(ConfigError::Validation(
        "server.listen is empty".to_string(),
      ));
    }
    if self.server.cache_priority < 1 {
      return Err(ConfigError::Validation(format!(
        "server.cache_priority must be >= 1, got {}",
        self.server.cache_priority
      )));
    }
    if self.server.read_timeout.0.is_zero() {
      return Err(ConfigError::Validation(
        "server.read_timeout must be positive".to_string(),
      ));
    }
    if self.server.write_timeout.0.is_zero() {
      return Err(ConfigError::Validation(
        "server.write_timeout must be positive".to_string(),
      ));
    }
    if self.cache.latency_alpha <= 0.0 || self.cache.latency_alpha >= 1.0 {
      return Err(ConfigError::Validation(format!(
        "cache.latency_alpha must be between 0 and 1 exclusive, got {}",
        self.cache.latency_alpha
      )));
    }
    if self.cache.ttl.0.is_zero() {
      return Err(ConfigError::Validation(
        "cache.ttl must be positive".to_string(),
      ));
    }
    if self.cache.negative_ttl.0.is_zero() {
      return Err(ConfigError::Validation(
        "cache.negative_ttl must be positive".to_string(),
      ));
    }
    if self.cache.max_entries <= 0 {
      return Err(ConfigError::Validation(
        "cache.max_entries must be positive".to_string(),
      ));
    }
    if self.cache.mass_query.max_concurrent_races == 0 {
      return Err(ConfigError::Validation(
        "cache.mass_query.max_concurrent_races must be >= 1".to_string(),
      ));
    }
    if self.cache.mass_query.per_upstream_max_inflight == 0 {
      return Err(ConfigError::Validation(
        "cache.mass_query.per_upstream_max_inflight must be >= 1".to_string(),
      ));
    }
    if self.cache.mass_query.in_memory_negative_ttl.0.is_zero() {
      return Err(ConfigError::Validation(
        "cache.mass_query.in_memory_negative_ttl must be positive".to_string(),
      ));
    }
    if self.cache.mass_query.upstream_cooldown.0.is_zero() {
      return Err(ConfigError::Validation(
        "cache.mass_query.upstream_cooldown must be positive".to_string(),
      ));
    }
    if self.mesh.enabled && self.mesh.peers.is_empty() {
      return Err(ConfigError::Validation(
        "mesh.enabled is true but no peers configured".to_string(),
      ));
    }
    for (i, peer) in self.mesh.peers.iter().enumerate() {
      if peer.addr.is_empty() {
        return Err(ConfigError::Validation(format!(
          "mesh.peers[{i}]: addr is empty"
        )));
      }
      if !peer.public_key.is_empty() {
        let bytes = hex::decode(&peer.public_key).map_err(|_| {
          ConfigError::Validation(format!(
            "mesh.peers[{i}]: public_key must be a hex-encoded 32-byte \
             ed25519 key"
          ))
        })?;
        if bytes.len() != 32 {
          return Err(ConfigError::Validation(format!(
            "mesh.peers[{i}]: public_key must be a hex-encoded 32-byte \
             ed25519 key"
          )));
        }
      }
    }
    if self.discovery.enabled {
      if self.discovery.service_name.is_empty() {
        return Err(ConfigError::Validation(
          "discovery.service_name is required when discovery is enabled"
            .to_string(),
        ));
      }
      if self.discovery.domain.is_empty() {
        return Err(ConfigError::Validation(
          "discovery.domain is required when discovery is enabled".to_string(),
        ));
      }
      if self.discovery.discovery_time.0.is_zero() {
        return Err(ConfigError::Validation(
          "discovery.discovery_time must be positive".to_string(),
        ));
      }
    }
    Ok(())
  }
}
