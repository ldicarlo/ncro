{
  lib,
  buildGoModule,
}:
buildGoModule (finalAttrs: {
  pname = "ncro";
  version = "1.0.0";

  src = let
    fs = lib.fileset;
    s = ../.;
  in
    fs.toSource {
      root = s;
      fileset = fs.unions [
        (s + /cmd)
        (s + /internal)
        (s + /go.mod)
        (s + /go.sum)
      ];
    };

  vendorHash = "sha256-9OkQIj2g5mZ+IpjIKvy8Il7J4xL4PJimEsXJP10FhmU=";
  ldflags = ["-s" "-w" "-X main.version=${finalAttrs.version}"];

  meta = {
    mainProgram = "ncro";
    maintainers = with lib.maintainers; [NotAShelf];
  };
})
