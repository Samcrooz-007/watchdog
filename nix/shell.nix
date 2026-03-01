{
  mkShell,
  go,
  gopls,
  golines,
  delve,
}:
mkShell {
  name = "go";
  packages = [
    go
    gopls # formatter
    golines # line wrapper
    delve # debugger
  ];
}
