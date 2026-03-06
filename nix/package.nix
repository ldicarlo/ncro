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

  vendorHash = "sha256-rjgb/iSz3+GPu8lNIhlTCwC/9uuuSh/PJv9GxvL7gJE=";

  ldflags = ["-s" "-w"];
}
