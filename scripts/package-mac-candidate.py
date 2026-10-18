#!/usr/bin/env python3
"""Package an already-built Mac core. No user data, credentials, or launch."""
import hashlib
import json
import os
from pathlib import Path
import platform
import plistlib
import re
import subprocess
import tempfile


def run(*args: str) -> None:
    subprocess.run(args, check=True)


def main() -> None:
    sha = os.environ.get("CANDIDATE_SHA", "")
    if platform.system() != "Darwin" or not re.fullmatch(r"[0-9a-f]{40}", sha):
        raise SystemExit("Mac host and exact CANDIDATE_SHA required")
    actual = subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip()
    if actual != sha:
        raise SystemExit("source revision mismatch")
    root = Path("build/bin")
    app = root / ("CST Mac " + sha[:8] + ".app")
    executables = app / "Contents/MacOS"
    executables.mkdir(parents=True, exist_ok=False)
    core = executables / "cst-core"
    core.write_bytes((root / "cst-core").read_bytes())
    core.chmod(0o755)
    # Native launcher supplies safe defaults even when Finder starts the bundle
    # without arguments. Explicit --data-dir arguments still override the default.
    launcher = r'''
#include <mach-o/dyld.h>
#include <limits.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
int main(int argc, char **argv) {
    char image[PATH_MAX], resolved[PATH_MAX], core[PATH_MAX], data[PATH_MAX];
    uint32_t size = sizeof(image);
    if (_NSGetExecutablePath(image, &size) || !realpath(image, resolved)) return 1;
    char *slash = strrchr(resolved, '/');
    if (!slash) return 1;
    *slash = 0;
    if (snprintf(core, sizeof(core), "%s/cst-core", resolved) >= sizeof(core)) return 1;
    const char *override = getenv("CST_CANDIDATE_DATA_DIR");
    const char *home = getenv("HOME");
    if (override && override[0]) {
        if (override[0] != '/' || strlcpy(data, override, sizeof(data)) >= sizeof(data)) return 1;
    } else {
        if (!home || home[0] != '/' || snprintf(data, sizeof(data),
            "%s/Library/Application Support/Clash-SpeedTest-Candidates/REVISION", home) >= sizeof(data)) return 1;
    }
    char **args = calloc((size_t)argc + 5, sizeof(char *));
    if (!args) return 1;
    args[0] = core; args[1] = "--gui"; args[2] = "--no-auto-credentials";
    args[3] = "--data-dir"; args[4] = data;
    for (int i = 1; i < argc; i++) args[i + 4] = argv[i];
    execv(core, args);
    perror("CST candidate launch");
    free(args);
    return 1;
}
'''.replace("REVISION", sha[:8])
    with tempfile.TemporaryDirectory(prefix="cst-launcher-") as temporary:
        source = Path(temporary) / "launcher.c"
        source.write_text(launcher)
        run("cc", "-Os", str(source), "-o", str(executables / "CST-Candidate"))
    with (app / "Contents/Info.plist").open("wb") as output:
        plistlib.dump({"CFBundleName": "CST Mac Candidate", "CFBundleDisplayName": "CST Mac " + sha[:8],
                      "CFBundleIdentifier": "com.clashspeedtest.candidate", "CFBundleExecutable": "CST-Candidate",
                      "CFBundlePackageType": "APPL", "CFBundleVersion": "1", "CFBundleShortVersionString": "1.0",
                      "NSHighResolutionCapable": True}, output)
    run("codesign", "--force", "--sign", "-", str(core))
    core_hash = hashlib.sha256(core.read_bytes()).hexdigest()
    manifest = {"source_commit": sha, "os": "darwin", "architecture": platform.machine(),
                "core_sha256": core_hash, "signature": "ad-hoc; not Developer ID notarization",
                "native_runtime_acceptance": "NOT_PERFORMED", "live_node_probes": 0,
                "default_data_root": "new isolated candidate directory; no automatic credentials"}
    (app / "Contents/build-manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
    run("codesign", "--force", "--sign", "-", str(app))
    run("codesign", "--verify", "--deep", "--strict", str(app))
    if hashlib.sha256(core.read_bytes()).hexdigest() != core_hash:
        raise SystemExit("core changed during bundle signing")
    run("ditto", "-c", "-k", "--sequesterRsrc", "--keepParent", str(app), str(root / "CST-Mac-candidate.zip"))
    print(json.dumps(manifest, indent=2))


if __name__ == "__main__":
    main()
