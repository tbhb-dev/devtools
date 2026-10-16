1. Confirm the pull request is mergeable
2. Settle the published description
3. Write the squash message in SQUASH_AGENTMSG
4. Run the commit-msg gates
5. Review the message with the review script
6. Confirm the message with the operator
7. Merge and clean up

Stop before any of it where preflight reports a missing precondition, a draft pull request, or a failing check. Say what's wrong and hand back.
