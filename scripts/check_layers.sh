#!/usr/bin/env bash
# Fails when a package imports something its layer must not (see "Project structure" in README.md).
# Only non-test imports are checked. Run: make check-layers
set -euo pipefail

mod=$(go list -m)

violations=$(go list -f '{{.ImportPath}} {{join .Imports " "}}' ./... | awk -v mod="$mod" '
BEGIN {
  n = 0
  # kind "only": the package may import only the listed in-repo packages (regex). kind "not": it must not import these.
  kind[n]="only"; pkg[n]="^domain$";                  rx[n]="^constants$";                                   n++
  kind[n]="only"; pkg[n]="^apperror$";                rx[n]="^constants$";                                   n++
  kind[n]="only"; pkg[n]="^constants$";               rx[n]="^$";                                            n++
  kind[n]="only"; pkg[n]="^config$";                  rx[n]="^$";                                            n++
  kind[n]="not";  pkg[n]="^repository(/|$)";         rx[n]="^(http|usecase)(/|$)";                          n++
  kind[n]="not";  pkg[n]="^usecase(/|$)";             rx[n]="^http(/|$)";                                    n++
  kind[n]="not";  pkg[n]="^http/controller$";         rx[n]="^(repository|driver/db)(/|$)";                  n++
  kind[n]="not";  pkg[n]="^http/(request|response)$"; rx[n]="^(usecase|repository|driver|api)(/|$)";        n++
  kind[n]="not";  pkg[n]="^http/(middleware|router)$"; rx[n]="^(usecase|repository)(/|$)";                  n++
  kind[n]="not";  pkg[n]="^(driver|api)(/|$)";        rx[n]="^(usecase|http|repository)(/|$)";               n++
}
{
  p = $1; sub("^" mod "/?", "", p); if (p == "") p = "."
  for (i = 2; i <= NF; i++) {
    imp = $i
    if (index(imp, mod "/") != 1) continue          # not a project package
    sub("^" mod "/", "", imp)
    for (r = 0; r < n; r++) {
      if (p !~ pkg[r]) continue
      if (kind[r] == "not" && imp ~ rx[r]) print p " must not import " imp
      if (kind[r] == "only" && imp !~ rx[r]) print p " may only import " rx[r] ", not " imp
    }
  }
}')

if [ -n "$violations" ]; then
  echo "layer violations:" >&2
  echo "$violations" | sed 's/^/  /' >&2
  exit 1
fi

# Project packages are imported without aliases (a redundant alias only hides which package is meant).
aliased=$(grep -rn -E '^[[:space:]]+[A-Za-z][A-Za-z0-9_]* "'"$mod"'/' --include='*.go' . || true)
if [ -n "$aliased" ]; then
  echo "aliased project imports:" >&2
  echo "$aliased" | sed 's/^/  /' >&2
  exit 1
fi
echo "layers ok"
