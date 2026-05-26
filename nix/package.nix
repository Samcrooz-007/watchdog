{
  lib,
  buildGoModule,
}: let
  versionInfo = lib.importJSON ../version.json;
in
  buildGoModule (finalAttrs: {
    pname = "watchdog";
    version = versionInfo.version;

    src = let
      fs = lib.fileset;
      s = ../.;
    in
      fs.toSource {
        root = s;
        fileset = fs.unions [
          (s + /cmd)
          (s + /internal)
          (s + /web)

          (s + /go.mod)
          (s + /go.sum)

          # Checkphase
          (s + /test)
          (s + /testdata)
          (s + /version.json)
        ];
      };

    vendorHash = "sha256-778YiWvdfdFtUrEiYLhBONSxjK6caq16LXxQcsHM3Mw=";

    ldflags = [
      "-s"
      "-w"
      "-X main.Version=${finalAttrs.version}"
      "-X main.Commit=${versionInfo.commit}"
      "-X main.BuildDate=${versionInfo.buildDate}"
    ];

    # Copy web assets
    postInstall = ''
      mkdir -p $out/share/watchdog
      cp -r $src/web $out/share/watchdog/
    '';

    meta = {
      description = "Privacy-preserving web analytics with Prometheus-native metrics";
      homepage = "https://github.com/notashelf/watchdog";
      license = lib.licenses.eupl12;
      maintainers = with lib.maintainers; [NotAShelf];
      mainProgram = "watchdog";
    };
  })
