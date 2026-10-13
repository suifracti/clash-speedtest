#!/usr/bin/env python3
"""Read-only source/remote/artifact checks. These are not functional acceptance."""
import argparse
import hashlib
import json
import subprocess
from pathlib import Path


def git(repo, *args):
    return subprocess.check_output(["git", "-C", str(repo), *args], text=True,
                                   stderr=subprocess.DEVNULL).strip()


def audit(repo, specification, *, base="origin/main", remote_head=None,
          app=None, manifest=None):
    repo = Path(repo)
    head = git(repo, "rev-parse", "HEAD")
    tree = git(repo, "rev-parse", "HEAD^{tree}")
    failures = []
    if git(repo, "status", "--porcelain"):
        failures.append("working_tree_dirty")
    if remote_head is None:
        failures.append("remote_not_checked")
    elif remote_head != head:
        failures.append("remote_head_mismatch")
    if subprocess.run(["git", "-C", str(repo), "merge-base", "--is-ancestor",
                       base, head], capture_output=True).returncode != 0:
        failures.append("base_not_in_candidate")
    features = []
    for feature in specification["features"]:
        absent = []
        for requirement in feature["required"]:
            path = repo / requirement["path"]
            text = path.read_text() if path.is_file() else ""
            if not path.is_file() or any(token not in text for token in requirement.get("contains", [])):
                absent.append(requirement["path"])
        features.append({"id": feature["id"], "source_present": not absent,
                         "missing_requirements": absent})
    missing = [f["id"] for f in features if not f["source_present"]]
    if missing:
        failures.append("accepted_capabilities_missing")
    source_ready = not failures
    artifact = {"state": "NOT_CHECKED"}
    if app is not None:
        app = Path(app)
        if not app.is_file() or manifest is None:
            artifact = {"state": "UNATTESTED"}
            failures.append("app_manifest_missing")
        else:
            artifact = {"state": "MANIFEST_MATCH", "core_sha256": hashlib.sha256(app.read_bytes()).hexdigest()}
            if manifest.get("source_revision") != head or manifest.get("source_tree") != tree:
                failures.append("app_source_revision_mismatch")
            if artifact["core_sha256"] != manifest.get("core_sha256"):
                failures.append("app_binary_hash_mismatch")
            if manifest.get("build_verified") is not True or not manifest.get("frontend_dist_sha256"):
                failures.append("app_build_unverified")
            if any(f.startswith("app_") for f in failures):
                artifact["state"] = "MISMATCH"
    return {"head": head, "tree": tree, "base": base, "remote_head": remote_head,
            "source_ready": source_ready, "full_candidate_ready": source_ready and app is not None and not failures,
            "features": features, "missing_features": missing, "app": artifact,
            "runtime_acceptance": "NOT_CHECKED", "failures": failures}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", type=Path, default=Path(__file__).resolve().parents[1])
    parser.add_argument("--base", default="origin/main")
    parser.add_argument("--branch", help="Exact GitHub candidate branch; defaults to configured upstream")
    parser.add_argument("--online", action="store_true", help="Fetch main and verify the exact remote branch; never pushes or edits files")
    parser.add_argument("--app", type=Path, help="Optional actual cst-core executable, not an App directory")
    parser.add_argument("--manifest", type=Path, help="Manifest produced during a clean, freshly rebuilt frontend/native build")
    parser.add_argument("--json-output", type=Path)
    args = parser.parse_args()
    try:
        remote_head = None
        if args.online:
            branch = args.branch or git(args.repo, "rev-parse", "--abbrev-ref", "@{upstream}").removeprefix("origin/")
            if not branch or branch.startswith("-") or any(c.isspace() for c in branch):
                raise ValueError("invalid candidate branch")
            subprocess.run(["git", "-C", str(args.repo), "fetch", "origin", "main"],
                           check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            ref = git(args.repo, "ls-remote", "--heads", "origin", "refs/heads/" + branch)
            remote_head = ref.split()[0] if ref else "MISSING_REMOTE_BRANCH"
        spec = json.loads((args.repo / "docs/candidate-capabilities.json").read_text())
        manifest = json.loads(args.manifest.read_text()) if args.manifest else None
        result = audit(args.repo, spec, base=args.base, remote_head=remote_head,
                       app=args.app, manifest=manifest)
    except (OSError, ValueError, subprocess.CalledProcessError):
        result = {"source_ready": False, "full_candidate_ready": False,
                  "runtime_acceptance": "NOT_CHECKED", "failures": ["audit_input_or_git_unavailable"]}
    text = json.dumps(result, ensure_ascii=False, indent=2)
    if args.json_output:
        args.json_output.write_text(text + "\n")
    print(text)
    return 0 if result["source_ready"] and (args.app is None or result["full_candidate_ready"]) else 1


if __name__ == "__main__":
    raise SystemExit(main())
