#!/bin/sh
# Fails unless every statement in the profile is covered.
#
# `go tool cover -func` reports a rounded percentage, so a handful of uncovered statements
# in a large tree still prints "100.0%" and slips through a threshold comparison. This
# counts statements instead, and names what is missing.
#
# Blocks are merged by position first: with -coverpkg the same block is reported once per
# test package, covered in one and not in another, so judging lines individually would call
# covered code uncovered.
set -eu

profile="${1:-coverage.out}"

awk '
  !/^mode:/ { stmts[$1] = $2; hits[$1] += $3 }
  END {
    for (block in stmts) {
      total += stmts[block]
      if (hits[block] > 0) {
        covered += stmts[block]
      } else {
        missing[++n] = block
      }
    }
    if (total == 0) {
      print "coverage gate: profile has no statements"
      exit 1
    }
    printf "statements covered: %d/%d (%.4f%%)\n", covered, total, 100 * covered / total
    if (n == 0) exit 0
    printf "\n%d uncovered block(s):\n", n
    for (i = 1; i <= n; i++) print "  " missing[i]
    exit 1
  }
' "$profile"
