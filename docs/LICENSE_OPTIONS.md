# Project license decision

PhotoSheet now uses Apache-2.0, with Copyright 2026 HugehardJiang in LICENSE. The optional native RAW dependency boundary remains a separate redistribution review and is not changed by the project license.

## Candidates

| Candidate | Advantages | Trade-offs |
| --- | --- | --- |
| Apache-2.0 | Permissive; includes an express patent license and contribution terms. | Longer notice and patent-termination provisions. |
| MIT | Very short and familiar; easy for application code and examples. | Less explicit patent language and fewer contribution terms. |
| MPL-2.0 | File-level copyleft can preserve changes to covered files while allowing larger proprietary combinations. | More notice/compliance work than MIT or Apache-2.0. |
| GPL-3.0 | Strong copyleft and clear user freedoms. | Would affect distribution choices and needs a deliberate project-wide policy. It does not automatically solve proprietary SDK terms. |

The permissive dependencies used by the core application do not force a copyleft license. The native Adobe DNG SDK is a separate issue: no PhotoSheet license can override its terms. The maintainer must review the exact SDK package and the exact native artifacts before distributing them.

Apache-2.0 was selected because it provides a permissive license with an express patent grant and contribution terms. The license selection does not approve redistribution of any separately licensed native SDK.
