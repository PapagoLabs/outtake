---
title: Users and Limits
description: Let the people you share your Plex server with use Outtake, choose who manages it, and set how much each member may render
weight: 4
---

Outtake belongs to one owner, the Plex account that owns the server Outtake clips from. Anyone you share that Plex server with can ask to use Outtake too.

## Roles

| Role | What it can do |
| --- | --- |
| **Owner** | Everything, including making members admins and removing admins. There is one owner |
| **Admin** | Approve and reject new accounts, remove members, set limits, and use every settings page. Admins have no render limits |
| **Member** | Make, preview, download, and delete their own clips, within their limits |
| **Pending** | Wait for approval. A pending account sees only a page saying so |

Each person sees only their own clips. The owner and admins see everyone's.

## Letting someone in

1. Share your Plex server with their Plex account in Plex, as you would for watching.
2. They open Outtake and log in with Plex. Outtake checks that your server is shared with them, then shows them **Waiting for Approval**. An account your server is not shared with is turned away with "This Plex account can't use this Outtake".
3. Under **Settings → Users**, choose **Approve** to let them in as a member, or **Reject** to turn them away. A rejected account can ask again by logging in.

A member browses your server as Plex shares it with them: only the libraries you shared, through their own Plex token. They can clip only titles they can see.

## Removing someone

**Remove** on **Settings → Users** ends their sessions at once. Their clips are kept, and the owner and admins still see them.

Unsharing your Plex server with someone ends their sessions too, within 10 minutes, and they cannot log in again until you share it again. Outtake checks every account's access each time it logs in and every 10 minutes while it is signed in.

## Limits

Every member has render limits. The owner and admins have none.

| Limit | What it counts | Default |
| --- | --- | --- |
| **Concurrent Renders** | Clips waiting or rendering, and previews rendering | 2 |
| **Renders per Week** | Clips started in any 7 days, counting regenerates and type changes | 100 |
| **Storage (GB)** | What the member's clips take, with their SDR versions | 25 |
| **Longest Clip (seconds)** | The longest clip the member may make, never longer than the installation's maximum | The installation's maximum |

**Limits for Members** on **Settings → Users** sets them for every member. Leave a field empty for no limit. An empty **Longest Clip** uses the installation's maximum.

Each member's own limits override the ones for every member. Leave a member's field empty to use the limit for every member, or enter 0 for no limit.

A clip or preview over a limit is refused with a message naming the limit, such as "You've reached your limit of 2 concurrent renders. Wait for one to finish." A weekly limit also says when the next render may start.
