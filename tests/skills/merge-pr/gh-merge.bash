gh_merge() {
  printf '%s\n' "${args[*]}" >"$F/merged"
  if [ "${REAL_MERGE:-0}" = 1 ]; then
    subject=""
    body=""
    for ((i = 0; i < ${#args[@]}; i++)); do
      case ${args[i]} in
      --subject) subject=${args[i + 1]} ;;
      --body-file) body=${args[i + 1]} ;;
      esac
    done
    printf '%s\n\n' "$subject" >"$F/message"
    cat "$body" >>"$F/message"
    [ "${WRONG_MERGE_MESSAGE:-0}" != 1 ] || echo wrong >"$F/message"
    merged_oid=$(git -C "$F/upstream.git" commit-tree "$(git rev-parse HEAD^{tree})" -p "$(git -C "$F/upstream.git" rev-parse main)" <"$F/message")
    git -C "$F/upstream.git" update-ref refs/heads/main "$merged_oid"
    printf '{"object":{"sha":"%s"}}\n' "$merged_oid" >"$F/api/repos_{owner}_{repo}_git_ref_heads_main.json"
    jq --arg oid "$merged_oid" '.state="MERGED" | .mergeCommit={oid:$oid}' "$F/pr.json" >"$F/next.json"
    mv "$F/next.json" "$F/pr.json"
    if [ "${ADVANCE_REMOTE_AFTER_MERGE:-0}" = 1 ]; then
      newer=$(echo later | git -C "$F/upstream.git" commit-tree "$(git rev-parse HEAD^{tree})" -p "$(git -C "$F/upstream.git" rev-parse topic)")
      git -C "$F/upstream.git" update-ref refs/heads/topic "$newer"
    fi
    exit 0
  fi
  case ${MERGE_RESULT:-merged} in
  merged | lost | missing-commit | wrong-head)
    jq '.state = "MERGED" | .mergeCommit = {oid:"4444444444444444444444444444444444444444"}' "$F/pr.json" >"$F/next.json"
    mv "$F/next.json" "$F/pr.json"
    case ${MERGE_RESULT:-merged} in
    lost)
      echo 'response lost' >&2
      exit 1
      ;;
    missing-commit)
      jq '.mergeCommit = null' "$F/pr.json" >"$F/next.json"
      mv "$F/next.json" "$F/pr.json"
      ;;
    wrong-head)
      jq '.headRefOid = "different"' "$F/pr.json" >"$F/next.json"
      mv "$F/next.json" "$F/pr.json"
      ;;
    esac
    ;;
  queued) ;;
  refused)
    echo 'merge refused' >&2
    exit 1
    ;;
  unavailable)
    echo 'read-back unavailable' >"$F/live-error"
    echo 1 >"$F/live-status"
    ;;
  esac
}
