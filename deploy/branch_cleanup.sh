#!/bin/bash
# Branch cleanup (human s94-online, 2026-11-06; CLAUDE.md clean-branch rule). Run by .github/workflows/branch-cleanup.yml
# (workflow_dispatch; session containers cannot delete refs). Arg 2 = dir with merged.txt / archive.txt ("<branch> <sha>").
#  merged.txt : branch fully in ffc -> delete.   archive.txt : has commits not in ffc -> tag archive/<b>, verify, delete.
# A branch whose head moved since it was listed (someone is using it) is skipped. Arg 1: "true" = dry run.
set -uo pipefail
DRY=${1:-true}
cd "${2:?list dir}"
git fetch -q --prune origin '+refs/heads/*:refs/remotes/origin/*' '+refs/tags/*:refs/tags/*'
del=(); tags=(); skip=0
while read -r b sha; do
  [[ -z "$b" || "$b" == \#* ]] && continue
  cur=$(git rev-parse -q --verify "refs/remotes/origin/$b") || { echo "gone   $b"; continue; }
  [ "$cur" = "$sha" ] || { echo "MOVED  $b (skip)"; skip=$((skip+1)); continue; }
  if [ "$(git rev-list --count origin/ffc.."$cur")" = 0 ]; then del+=("$b"); else echo "AHEAD  $b -> archive"; tags+=("$b"); fi
done < merged.txt
while read -r b sha; do
  [[ -z "$b" || "$b" == \#* ]] && continue
  cur=$(git rev-parse -q --verify "refs/remotes/origin/$b") || { echo "gone   $b"; continue; }
  [ "$cur" = "$sha" ] || { echo "MOVED  $b (skip)"; skip=$((skip+1)); continue; }
  tags+=("$b")
done < archive.txt
echo "delete-only ${#del[@]}, archive+delete ${#tags[@]}, skipped $skip, dry=$DRY"
[ "$DRY" = "true" ] && exit 0
ok=()
for b in "${tags[@]}"; do git tag -f "archive/$b" "refs/remotes/origin/$b" >/dev/null; done
printf 'refs/tags/archive/%s\n' "${tags[@]}" | xargs -r -n 50 git push -q origin
git fetch -q origin '+refs/tags/archive/*:refs/tags/archive/*'
for b in "${tags[@]}"; do
  [ "$(git ls-remote origin "refs/tags/archive/$b" | cut -f1)" = "$(git rev-parse "refs/remotes/origin/$b")" ] && ok+=("$b") || echo "TAG FAILED $b (kept)"
done
all=("${del[@]}" "${ok[@]}")
printf '%s\n' "${all[@]}" | xargs -r -n 50 git push -q origin --delete
git fetch -q --prune origin
left=0; for b in "${all[@]}"; do git rev-parse -q --verify "refs/remotes/origin/$b" >/dev/null && { echo "STILL  $b"; left=$((left+1)); }; done
echo "done: deleted $(( ${#all[@]} - left )), archived ${#ok[@]}, not deleted $left"
