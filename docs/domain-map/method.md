# Domain map method

<!-- cspell:words fnmatchcase timedelta isoformat splitlines fnmatch itertools lineterminator writerow writerows getvalue breleas bagy defaultdict findall hashlib hexdigest -->

This is a read-only snapshot for [#553](https://github.com/tbhb-dev/devtools/issues/553). Run the commands from the repository root with Python and authenticated `gh`. The analysis uses read-only queries.

## Inputs and rules

The snapshot, `input.json.gz`, records the collection time, window endpoints, base commit, tracked paths, open issues, and merged and open pull requests. Compression reduces the space for repeated historical paths. Collection isn't an atomic GitHub transaction. Open work can change while collection runs. The recorded data, rather than live queries, defines the report.

Domain rules appear in priority order in `rules.json`. Python `fnmatchcase` matches the whole path, with `*` matching slashes too. A match assigns the domain at the first applicable glob. Release paths take precedence over workflow paths. The final shared rule classifies everything else, including root configuration, documentation, and repository tooling. Deleted historical paths use these same rules. Vendored Go sources count toward Go size. They don't measure review effort.

For issues, a named tracked path or directory in the body supplies the strongest signal. Next come label keywords, then title keywords. Every match appears in the output as evidence. Each domain keeps its strongest signal. Ties follow the order of domains in the rules. A primary domain has the strongest signal, and all other matches are secondary. No match means shared. For directories, the command searches for literal prefixes from tracked paths. This mechanical mapping needs human judgement because broad titles and references to shared documents can move work into shared. Body prose without a named path doesn't supply a keyword signal. Extracted paths replace whole issue bodies in the snapshot.

## Collect a fresh snapshot

Fetch origin first so merged commit objects exist locally. This command replaces the snapshot. It doesn't reproduce the original observation time. GitHub pagination collects all open issues and all merged or open pull requests. Only merges within the recorded window enter the analysis. A merged pull request's paths come from its merge commit against its first parent, without rename detection. Open paths come from the paginated pull request files API. That API reports the destination path for a rename. The conflict view compares current path names. A missing local merge object fails collection.

```sh
git fetch origin
python3 - <<'PY'
import json, subprocess, datetime
from pathlib import Path

def run(*args):
    return subprocess.check_output(args, text=True)

query = '''query($endCursor: String) { repository(owner:"tbhb-dev",name:"devtools") { pullRequests(first:100,after:$endCursor,states:[MERGED,OPEN],orderBy:{field:CREATED_AT,direction:ASC}) { nodes { number title state mergedAt mergeCommit { oid } baseRefOid headRefOid } pageInfo { hasNextPage endCursor } } } }'''
pages = run('gh','query','graphql','--paginate','-f','query='+query)
decoder = json.JSONDecoder()
prs=[]
while pages.strip():
    obj, end = decoder.raw_decode(pages.lstrip())
    pages=pages.lstrip()[end:]
    prs.extend(obj['data']['repository']['pullRequests']['nodes'])
issues_text=run('gh','query','--paginate','repos/tbhb-dev/devtools/issues?state=open&per_page=100','--jq','.[] | select(.pull_request == null) | {number,title,body,labels:[.labels[].name]}')
decoder=json.JSONDecoder(); issues=[]
while issues_text.strip():
    obj,end=decoder.raw_decode(issues_text.lstrip()); issues_text=issues_text.lstrip()[end:]; issues.append(obj)
base=run('git','rev-parse','HEAD').strip()
paths=run('git','ls-tree','-rz','--name-only',base).split('\0')[:-1]
now=datetime.datetime.now(datetime.timezone.utc).replace(microsecond=0)
start=now-datetime.timedelta(days=90)
merged=[]; opened=[]
for pr in prs:
    if pr['state']=='MERGED' and start.isoformat() <= pr['mergedAt'] <= now.isoformat():
        sha=pr['mergeCommit']['oid']
        pr['paths']=run('git','diff','--name-only','--no-renames',sha+'^',sha).splitlines()
        merged.append(pr)
    elif pr['state']=='OPEN':
        pr['paths']=[f['filename'] for page in json.loads(run('gh','query','--paginate','--slurp',f"repos/tbhb-dev/devtools/pulls/{pr['number']}/files?per_page=100")) for f in page]
        opened.append(pr)
for issue in issues:
    body=issue.pop('body') or ''
    issue['body_paths']=[p for p in paths if p in body]
    # Record directory references as well as individual tracked paths.
    issue['body_directories']=sorted({p[:i+1] for p in paths for i,c in enumerate(p) if c=='/' and p[:i+1] in body})
out={'collected_at':now.isoformat(),'window_start':start.isoformat(),'window_end':now.isoformat(),'base':base,'tracked_paths':paths,'issues':sorted(issues,key=lambda x:x['number']),'merged_prs':sorted(merged,key=lambda x:x['number']),'open_prs':sorted(opened,key=lambda x:x['number'])}
Path('docs/domain-map').mkdir(exist_ok=True)
import gzip
Path('docs/domain-map/input.json.gz').write_bytes(gzip.compress((json.dumps(out,indent=2,sort_keys=True)+'\n').encode(),mtime=0))
print({k:len(out[k]) for k in ['tracked_paths','issues','merged_prs','open_prs']})
PY
```

## Regenerate and check offline

Extract and run the Python block below from the repository root. It writes `paths.csv`, `issues.csv`, `merged-paths.csv`, `pairs.csv`, `open-conflicts.csv`, `domains.csv`, and `totals.csv`. Detailed tables use `gzip` compression to reduce repeated paths. Read them with `gzip -dc`:

```sh
gzip -dc docs/domain-map/open-conflicts.csv.gz
```

Each CSV has a header. Open conflicts group pull request numbers by shared path. Overlap doesn't prove a textual conflict. Pairs count each unordered domain pair once per merged pull request. Each pull request contributes to every pair among the domains it changes. Boundary figures count merged pull requests touching a domain and at least one other domain, including shared.

```python
import collections
import csv
import fnmatch
import gzip
import itertools
import json
import re
from pathlib import Path

ROOT = Path("docs/domain-map")
DATA = json.loads(gzip.decompress((ROOT / "input.json.gz").read_bytes()))
RULES = json.loads((ROOT / "rules.json").read_text())
DOMAINS = list(RULES)


def domain(path):
    return next(
        name
        for name, globs in RULES.items()
        if any(fnmatch.fnmatchcase(path, glob) for glob in globs)
    )


def write(name, header, rows):
    import io

    stream = io.StringIO(newline="")
    writer = csv.writer(stream, lineterminator="\n")
    writer.writerow(header)
    writer.writerows(rows)
    content = stream.getvalue().encode()
    if name in {"paths.csv", "issues.csv", "merged-paths.csv", "open-conflicts.csv"}:
        (ROOT / (name + ".gz")).write_bytes(gzip.compress(content, mtime=0))
    else:
        (ROOT / name).write_bytes(content)


# Ordered first-match rules include a shared catch-all, including new paths.
assert DOMAINS == ["release", "skills", "go", "consumer", "workflows", "shared"]
assert RULES["shared"] == ["*"]
assert len(set(DATA["tracked_paths"])) == len(DATA["tracked_paths"])
assert all(domain(p) in DOMAINS for p in DATA["tracked_paths"])
import subprocess

base_paths = subprocess.check_output(
    ["git", "ls-tree", "-rz", "--name-only", DATA["base"]], text=True
).split("\0")[:-1]
assert base_paths == DATA["tracked_paths"]
current_paths = subprocess.check_output(["git", "ls-files", "-z"], text=True).split(
    "\0"
)[:-1]
assert all(domain(p) in DOMAINS for p in current_paths)
assert domain("packages/agents/source/lib/lib.sh") == "skills"
assert domain("cmd/agentrepo/main.go") == "go"
assert domain(".devtools/tasks/lint.toml") == "consumer"
assert domain(".github/workflows/release.yml") == "release"
assert domain(".github/workflows/test.yml") == "workflows"
assert domain("a-new-root-file") == "shared"
write("paths.csv", ["path", "domain"], ([p, domain(p)] for p in DATA["tracked_paths"]))

# A body path or directory is strongest evidence; labels precede titles.
# Each domain receives its strongest signal, rather than a vote per path.
WORDS = {
    "release": r"\breleas(?:e|es|ing)\b|\bversion(?:ing)?\b|\bchangelog\b",
    "skills": r"\bskills?\b|\bagents?\b|\bcommit\b|\bsquash\b|\breview(?:er)?\b|\bpr\b",
    "go": r"\bgolang\b|\bgo\b|\bgh-post\b|\bagentrepo\b|\bagy-otel\b",
    "consumer": r"\bconsumer\b|\bmise\b|\blint\b|\bbuild\b|\bdevtools\b",
    "workflows": r"\bworkflow\b|\bactions\b|\bci\b|\brenovate\b",
}
issue_rows = []
for issue in DATA["issues"]:
    scores = collections.defaultdict(int)
    evidence = []
    for path in issue["body_paths"] + issue["body_directories"]:
        name = domain(path)
        scores[name] = 3
        evidence.append("path=" + path)
    for field, weight in [("labels", 2), ("title", 1)]:
        text = " ".join(issue[field]) if field == "labels" else issue[field]
        for name, pattern in WORDS.items():
            matches = sorted(set(re.findall(pattern, text.lower())))
            if matches:
                scores[name] = max(scores[name], weight)
                evidence.append(field + ":" + name + "=" + "|".join(matches))
    ranked = sorted(scores, key=lambda name: (-scores[name], DOMAINS.index(name)))
    if not ranked:
        ranked = ["shared"]
        evidence = ["fallback: no domain signal"]
    issue_rows.append(
        [issue["number"], ranked[0], "|".join(ranked[1:]), ";".join(sorted(evidence))]
    )
assert len({row[0] for row in issue_rows}) == len(DATA["issues"])
assert [row[0] for row in issue_rows] == sorted(row[0] for row in issue_rows)
write("issues.csv", ["issue", "primary", "secondary", "evidence"], issue_rows)

merged_rows = []
pairs = collections.Counter()
widths = collections.Counter()
cuts = collections.Counter()
for pr in DATA["merged_prs"]:
    touched = sorted({domain(p) for p in pr["paths"]})
    widths["single" if len(touched) == 1 else "cross"] += 1
    pairs.update(itertools.combinations(touched, 2))
    if len(touched) > 1:
        cuts.update(touched)
    for path in pr["paths"]:
        merged_rows.append(
            [pr["number"], pr["mergedAt"], pr["mergeCommit"]["oid"], domain(path), path]
        )
write("merged-paths.csv", ["pr", "merged_at", "commit", "domain", "path"], merged_rows)
write(
    "pairs.csv",
    ["domain_a", "domain_b", "prs"],
    ([a, b, n] for (a, b), n in sorted(pairs.items(), key=lambda x: (-x[1], x[0]))),
)

owners = collections.defaultdict(set)
for pr in DATA["open_prs"]:
    for path in pr["paths"]:
        owners[path].add(pr["number"])
conflicts = [
    [path, domain(path), "|".join(map(str, sorted(numbers)))]
    for path, numbers in sorted(owners.items())
    if len(numbers) > 1
]
write("open-conflicts.csv", ["path", "domain", "open_prs"], conflicts)
sizes = collections.Counter(domain(p) for p in DATA["tracked_paths"])
work = collections.Counter(row[1] for row in issue_rows)
write(
    "domains.csv",
    ["domain", "tracked_paths", "primary_issues", "boundary_prs"],
    ([name, sizes[name], work[name], cuts[name]] for name in DOMAINS),
)
write(
    "totals.csv",
    ["measure", "value"],
    sorted(
        {
            "single_domain_prs": widths["single"],
            "cross_domain_prs": widths["cross"],
            "open_issues": len(issue_rows),
            "merged_prs": len(DATA["merged_prs"]),
            "open_prs": len(DATA["open_prs"]),
            "shared_files_in_open_prs": len(conflicts),
        }.items()
    ),
)
print("Path rules, issue uniqueness, and domain examples passed.")
print(dict(widths))
print(pairs.most_common(8))
print("sizes:", dict(sizes), "work:", dict(work), "cuts:", dict(cuts))
```

To execute that block and verify byte-for-byte repeatability:

```sh
python3 - <<'PY'
import hashlib
import subprocess
from pathlib import Path

root = Path('docs/domain-map')
method = (root / 'method.md').read_text()
code = method.split('```python\n', 1)[1].split('```', 1)[0]
def digests():
    return {p.name: hashlib.sha256(p.read_bytes()).hexdigest()
            for p in sorted(root.glob('*.csv*'))}
expected = digests()
subprocess.run(['python3', '-c', code], check=True)
assert digests() == expected
subprocess.run(['python3', '-c', code], check=True)
assert digests() == expected
print('All recorded tables reproduce exactly.')
PY
```

Check the first, middle, and last merged pull requests against Git:

```sh
python3 - <<'PY'
import gzip
import json
import subprocess
from pathlib import Path

data = json.loads(gzip.decompress(Path('docs/domain-map/input.json.gz').read_bytes()))
prs = data['merged_prs']
for pr in [prs[0], prs[len(prs) // 2], prs[-1]]:
    sha = pr['mergeCommit']['oid']
    paths = subprocess.check_output(
        ['git', 'diff', '--name-only', '--no-renames', sha + '^', sha], text=True
    ).splitlines()
    assert paths == pr['paths']
    print(pr['number'], sha, 'matches git diff --name-only')
PY
```

Finish with `mise run lint`. The [report](../domain-map.md) draws its figures from these tables.
