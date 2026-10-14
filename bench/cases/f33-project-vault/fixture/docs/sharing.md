# Recipient links

Links are recipient-bound read capabilities with exclusive expiration. Editors may issue,
revoke or extend a link. Revocation is permanent for that token: renewing its expiry does
not restore access. A fresh issue creates a new token. Recipients do not need membership.
Inspection is limited to the issuing actor and returns a detached public summary.

Journal timestamps use the injected millisecond clock. Reading and preparing jobs emit no
mutation events. Document labels are unique, sorted strings; updates increment revisions.
