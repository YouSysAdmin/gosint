// Gosint, Copyright (c) 2026 YouSysAdmin

package username

// Site is one place a username may have a profile. URL holds {} where
// the username goes. A profile exists when the page answers 200, unless
// Absent is set and the page carries that text, which is how sites that
// answer 200 for everything say "no such user", or AbsentStatus is the
// status a missing profile gets instead of 404. Plain sends the
// server's own agent instead of a browser's, for a site whose bot
// check fires on browsers only (Codeberg's Anubis).
type Site struct {
	Name         string
	URL          string
	Absent       string
	AbsentStatus int
	Plain        bool
}

// walls are what a bot check answers in place of the page, with a 200:
// such an answer says nothing about the profile and is reported as no
// answer, never as a profile.
var walls = []string{"<title>Client Challenge", "<title>Just a moment...", "Attention Required! | Cloudflare", "<title>Access denied"}

// Builtin is the list shipped. It keeps to sites whose answer for a
// missing profile is an honest 404, a fixed sentence or a fixed status,
// which is what makes a probe worth trusting. Sites behind a sign-in
// or a bot wall for every client (Instagram, X, LinkedIn, TikTok,
// Medium, npm, Letterboxd, TryHackMe) are left out: they answer the
// same page, or a refusal, for everyone. Reddit wants a signed-in
// session for its JSON, Spotify serves a browser one shell for every
// name and anyone else a 404, Last.fm and PyPI answer a bot check to
// every server, so none of them can be asked.
var Builtin = []Site{
	{Name: "GitHub", URL: "https://github.com/{}"},
	{Name: "GitLab", URL: "https://gitlab.com/api/v4/users?username={}", Absent: "[]"},
	{Name: "Codeberg", URL: "https://codeberg.org/{}", Plain: true},
	{Name: "Bitbucket", URL: "https://bitbucket.org/{}/"},
	{Name: "Docker Hub", URL: "https://hub.docker.com/v2/users/{}/"},
	{Name: "Hacker News", URL: "https://news.ycombinator.com/user?id={}", Absent: "No such user."},
	{Name: "YouTube", URL: "https://www.youtube.com/@{}"},
	{Name: "DEV", URL: "https://dev.to/{}"},
	{Name: "Hashnode", URL: "https://hashnode.com/@{}", Absent: "User not found | Hashnode"},
	{Name: "Keybase", URL: "https://keybase.io/{}"},
	{Name: "Mastodon", URL: "https://mastodon.social/@{}"},
	{Name: "Mathstodon", URL: "https://mathstodon.xyz/@{}"},
	{Name: "Bluesky", URL: "https://public.api.bsky.app/xrpc/app.bsky.actor.getProfile?actor={}.bsky.social", AbsentStatus: 400},
	{Name: "Steam", URL: "https://steamcommunity.com/id/{}", Absent: "The specified profile could not be found"},
	{Name: "Lichess", URL: "https://lichess.org/@/{}"},
	{Name: "Chess.com", URL: "https://api.chess.com/pub/player/{}"},
	{Name: "SoundCloud", URL: "https://soundcloud.com/{}"},
	{Name: "Flickr", URL: "https://www.flickr.com/people/{}/"},
	{Name: "Vimeo", URL: "https://vimeo.com/{}"},
	{Name: "Behance", URL: "https://www.behance.net/{}"},
	{Name: "Dribbble", URL: "https://dribbble.com/{}"},
	{Name: "Patreon", URL: "https://www.patreon.com/{}"},
	{Name: "Linktree", URL: "https://linktr.ee/{}"},
	{Name: "About.me", URL: "https://about.me/{}"},
	{Name: "Gravatar", URL: "https://gravatar.com/{}"},
	{Name: "Kaggle", URL: "https://www.kaggle.com/{}"},
	{Name: "Replit", URL: "https://replit.com/@{}"},
	{Name: "Scratch", URL: "https://scratch.mit.edu/users/{}/"},
	{Name: "Wikipedia", URL: "https://en.wikipedia.org/wiki/User:{}"},
	{Name: "Pastebin", URL: "https://pastebin.com/u/{}"},
	{Name: "HackerOne", URL: "https://hackerone.com/{}"},
	{Name: "Bugcrowd", URL: "https://bugcrowd.com/{}"},
	{Name: "Snapchat", URL: "https://www.snapchat.com/add/{}"},
}
