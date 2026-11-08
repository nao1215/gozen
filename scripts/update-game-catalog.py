#!/usr/bin/env python3
"""Propose additive catalog changes from approved GitHub release metadata.

This does not approve releases, sign catalogs, or execute upstream content.
The Go catalog check validates every proposed archive before a bot opens a PR.
"""

import argparse
import json
import os
from pathlib import Path
import re
import urllib.error
import urllib.parse
import urllib.request


class HTTPSRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        if urllib.parse.urlsplit(newurl).scheme != "https":
            raise ValueError("release metadata redirected away from HTTPS")
        return super().redirect_request(req, fp, code, msg, headers, newurl)


def download(url, limit, token=""):
    if urllib.parse.urlsplit(url).scheme != "https":
        raise ValueError("metadata URL must use HTTPS")
    headers = {"Accept": "application/vnd.github+json", "User-Agent": "gozen-catalog-updater"}
    if token:
        if urllib.parse.urlsplit(url).netloc != "api.github.com":
            raise ValueError("GitHub credentials are only sent to its API")
        headers["Authorization"] = f"Bearer {token}"
    request = urllib.request.Request(url, headers=headers)
    with urllib.request.build_opener(HTTPSRedirect()).open(request, timeout=30) as response:
        data = response.read(limit + 1)
    if len(data) > limit:
        raise ValueError("metadata exceeds size limit")
    return json.loads(data)


def archive_url(url, repository):
    parsed = urllib.parse.urlsplit(url)
    prefix = f"/{repository}/releases/download/"
    suffix = parsed.path.removeprefix(prefix)
    return (
        parsed.scheme == "https" and parsed.netloc == "github.com"
        and not parsed.query and not parsed.fragment and "%" not in parsed.path
        and parsed.path.startswith(prefix) and len(suffix.split("/")) == 2
        and all(part and part not in (".", "..") for part in suffix.split("/"))
    )


def browser_url(url, repository):
    parsed = urllib.parse.urlsplit(url)
    prefix = f"/{repository}/"
    suffix = parsed.path.removeprefix(prefix)
    return (
        parsed.scheme == "https" and parsed.netloc == "raw.githubusercontent.com"
        and not parsed.query and not parsed.fragment and "%" not in parsed.path
        and parsed.path.startswith(prefix)
        and re.fullmatch(r"[0-9a-f]{40}/.+\.zip", suffix) is not None
        and all(part and part not in (".", "..") for part in suffix.split("/"))
    )


def candidates(catalog, sources, fetch):
    existing = {(r["id"], r["version"]): r for r in catalog["releases"]}
    result = list(catalog["releases"])
    for source in sources["sources"]:
        repository = source["repository"]
        if not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", repository):
            raise ValueError("invalid approved repository")
        if source["manifest_asset"] != "gozen-package.json":
            raise ValueError("unsupported release metadata asset")
        releases = fetch(f"https://api.github.com/repos/{repository}/releases?per_page=100", 4 << 20, True)
        if not isinstance(releases, list):
            raise ValueError("unexpected GitHub release response")
        for release in releases:
            if release.get("draft") or release.get("prerelease"):
                continue
            assets = [a for a in release.get("assets", []) if a["name"] == source["manifest_asset"]]
            if not assets:
                continue
            if len(assets) != 1 or not archive_url(assets[0]["browser_download_url"], repository):
                raise ValueError("metadata asset leaves the approved repository")
            metadata = fetch(assets[0]["browser_download_url"], 256 << 10, False)
            # A repository may publish several approved games under separate tags.
            # Only this source's identity is a candidate; other IDs gain no approval.
            if metadata["id"] != source["id"]:
                continue
            if not archive_url(metadata["url"], repository):
                raise ValueError("release leaves approved repository")
            if metadata.get("browser_url") and not browser_url(metadata["browser_url"], repository):
                raise ValueError("browser mirror must pin a commit in the approved repository")
            if not re.fullmatch(r"(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)", metadata["version"]):
                raise ValueError("release version must be stable major.minor.patch")
            # Both files must belong to the same immutable upstream release tag.
            if urllib.parse.urlsplit(metadata["url"]).path.rsplit("/", 1)[0] != urllib.parse.urlsplit(assets[0]["browser_download_url"]).path.rsplit("/", 1)[0]:
                raise ValueError("archive and metadata use different release tags")
            key = (metadata["id"], metadata["version"])
            if key in existing:
                if existing[key] != metadata:
                    raise ValueError(f"immutable release changed: {key}")
                continue
            existing[key] = metadata
            result.append(metadata)
    return {"schema": catalog["schema"], "releases": result}


def has_pending_update(pull_requests, repository):
    """Only this repository's update branches may suppress a scheduled proposal."""
    return any(
        pr.get("headRefName", "").startswith("automation/game-catalog-")
        and (pr.get("headRepository") or {}).get("nameWithOwner") == repository
        for pr in pull_requests
    )


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--catalog", default="docs/registry/catalog.json")
    parser.add_argument("--sources", default="docs/registry/sources.json")
    parser.add_argument("--pending-prs", help="GitHub JSON list of open PRs")
    parser.add_argument("--repository", help="Repository owning the updater branch")
    args = parser.parse_args()
    if args.pending_prs:
        if not args.repository:
            parser.error("--pending-prs requires --repository")
        if has_pending_update(json.loads(Path(args.pending_prs).read_text()), args.repository):
            print("An existing catalog update awaits review.")
            return
    catalog_path = Path(args.catalog)
    catalog = json.loads(catalog_path.read_text())
    sources = json.loads(Path(args.sources).read_text())
    token = os.environ.get("GH_TOKEN", "")
    updated = candidates(catalog, sources, lambda url, limit, api: download(url, limit, token if api else ""))
    # Write only after all metadata succeeds; validation and PR review follow.
    if updated != catalog:
        catalog_path.write_text(json.dumps(updated, indent=2, ensure_ascii=True) + "\n")
        print(f"Proposed {len(updated['releases']) - len(catalog['releases'])} releases")
    else:
        print("No new releases")


if __name__ == "__main__":
    main()
