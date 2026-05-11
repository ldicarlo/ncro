{
  lib,
  rustPlatform,
  pkg-config,
}:
rustPlatform.buildRustPackage (finalAttrs: {
  pname = "ncro";
  version = "1.0.0";

  src = let
    fs = lib.fileset;
    s = ../.;
  in
    fs.toSource {
      root = s;
      fileset = fs.unions [
        (s + /src)
        (s + /Cargo.toml)
        (s + /Cargo.lock)
      ];
    };

  cargoLock.lockFile = "${finalAttrs.src}/Cargo.lock";
  nativeBuildInputs = [pkg-config];

  meta = {
    mainProgram = "ncro";
    maintainers = with lib.maintainers; [NotAShelf];
  };
})
