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

  vendorHash = "sha256-vhCOK0cD92F9xMBS4APH+0nvLftaPuRl2LJio4mYWhY=";

  ldflags = ["-s" "-w"];
}
