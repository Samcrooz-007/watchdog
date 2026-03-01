{
  lib,
  buildGoModule,
}:
buildGoModule (finalAttrs: {
  pname = "watchdog";
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

  vendorHash = "sha256-jMqPVvMZDm406Gi2g4zNSRJMySLAN7/CR/2NgF+gqtA=";

  ldflags = ["-s" "-w" "-X main.version=${finalAttrs.version}"];

  # Copy web assets
  postInstall = ''
    mkdir -p $out/share/watchdog
    cp -r $src/web $out/share/watchdog/
  '';

  meta = {
    description = "Privacy-preserving web analytics with Prometheus-native metrics";
    homepage = "https://github.com/notashelf/watchdog";
    license = lib.licenses.gpl3Only;
    maintainers = with lib.maintainers; [NotAShelf];
    mainProgram = "watchdog";
  };
})
