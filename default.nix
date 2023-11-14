{ lib
, buildGo120Module
, fetchFromGitHub
}:

buildGo120Module rec {
  pname = "btrfs-rec-tools";
  version = "unstable-2023-07-25";

  src = fetchFromGitHub {
    owner = "bzm3r";
    repo = "btrfs-progs-ng";
    rev = "b3ef47542802ecd06498a2fc3a87ce26f7bc1e98";
    hash = "sha256-JuKC3VCB106v4qXej644av+q0RfhraEH7A2shV6BVZU=";
  };

  vendorHash = "sha256-HNmVGZ2lV8advPfS+691hHCj69zb2vZ9d1nLT0cWJh8=";

  ldflags = [ "-s" "-w" ];

  subPackages = [ "cmd/btrfs-rec" ];

  meta = with lib; {
    description = "Nix packaged version of: https://git.lukeshu.com/btrfs-progs-ng";
    homepage = "https://github.com/bzm3r/btrfs-progs-ng";
    license = with licenses; [ gpl2Only gpl3Only asl20 mpl20 ];
    maintainers = with maintainers; [ ];
    mainProgram = "btrfs-rec-tools";
  };
}
