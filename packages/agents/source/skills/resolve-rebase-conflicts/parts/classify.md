One call reads every unmerged path and says what shape its conflict has, with the evidence for the claim printed beside it. Read that output before opening any file.

The classes:

| Class | What it means | What to do |
| --- | --- | --- |
| `sorted-union` | All three versions sorted and unique, and neither side removed a line | `resolve-union.sh`, no confirmation |
| `regenerate` | The repository declares the path generated | `take-side.sh <path> base-side`, then run the generator after the rebase |
| `token-union` | Both sides extended the same line with extra tokens | Apply the printed proposal once the operator confirms the ordering |
| `content` | A real disagreement | Read what each side changed, then resolve by hand |

For a content conflict the script also prints what each side changed against the common ancestor. The markers can't show you that, and it's what settles the resolution.
