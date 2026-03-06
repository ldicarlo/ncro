{
  lib,
  buildGoModule,
}:
buildGoModule {
  pname = "ncro";
  version = "0.1.0";

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

  vendorHash = "sha256-suI8EAgRFG7BDJP2aqLWsej6FTP+OrEsmxRyV5hkKK0=";

  ldflags = ["-s" "-w"];
}
