{
  mkShell,
  go,
  gopls,
  golines,
  delve,
  golangci-lint,
}:
mkShell {
  name = "go";
  packages = [
    go
    gopls # formatter
    golines # line wrapper
    delve # debugger
    golangci-lint # linter
  ];
}
