{
  mkShell,
  go,
  gopls,
  delve,
  gofumpt,
  golines,
}:
mkShell {
  name = "go";
  packages = [
    delve
    go
    gopls
    gofumpt
    golines
  ];
}
