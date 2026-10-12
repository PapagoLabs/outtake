// live.js keeps a page current over one event stream.
//
// The page marks what the stream follows:
//   - a rendering clip card's status region carries data-clip-live with its
//     clip id, and the stream morphs each new region into it.
//   - the media page's playback panel carries data-playback-live with its
//     media id, and the stream morphs each new panel into it.
//   - a rendering preview's player carries data-preview-live with its preview
//     id, and the stream hands each new status to media-item.js as an
//     outtake:preview event.
//
// The stream sends a change when it happens, and a ping every second. While
// pings arrive, the polls of what it follows are held back. When they stop,
// as behind a proxy that buffers the stream, the polls carry on.
//
// The stream holds one of the browser's few connections to the server, so it
// is open only while the page is visible and follows something. A script that
// changes a marker itself dispatches outtake:live-sync so the stream follows.
(function () {
	'use strict';

	var streamPath = '/live';
	// maxClips is the most clips one stream follows, as the server caps it.
	// Cards past it keep their own polls.
	var maxClips = 50;
	// staleAfterMs is how long after the last event the stream counts as
	// silent, which is two and a half of its one-second checks.
	var staleAfterMs = 2500;

	var source = null;
	var following = '';
	var followed = { clip: {}, preview: {}, playback: {} };
	var lastEventAt = 0;

	function markedIds(attribute) {
		var ids = [];
		document.querySelectorAll('[' + attribute + ']').forEach(function (element) {
			ids.push(element.getAttribute(attribute));
		});

		return ids.sort();
	}

	// wanted lists what the page needs followed now, by feed name.
	function wanted() {
		if (document.hidden) {
			return {};
		}
		var feeds = {};
		var clips = markedIds('data-clip-live').slice(0, maxClips);
		var preview = markedIds('data-preview-live').slice(0, 1);
		var playback = markedIds('data-playback-live').slice(0, 1);
		if (clips.length) { feeds.clip = clips; }
		if (preview.length) { feeds.preview = preview; }
		if (playback.length) { feeds.playback = playback; }

		return feeds;
	}

	function queryOf(feeds) {
		return Object.keys(feeds).sort().map(function (name) {
			return name + '=' + encodeURIComponent(feeds[name].join(','));
		}).join('&');
	}

	function close() {
		if (source) {
			source.close();
			source = null;
		}
		following = '';
		followed = { clip: {}, preview: {}, playback: {} };
		lastEventAt = 0;
	}

	// sync opens, reopens, or closes the stream to follow what the page shows
	// now.
	function sync() {
		var feeds = wanted();
		var query = queryOf(feeds);
		if (query === following) {
			return;
		}
		close();
		if (!query) {
			return;
		}
		following = query;
		Object.keys(feeds).forEach(function (name) {
			feeds[name].forEach(function (id) { followed[name][id] = true; });
		});
		source = new EventSource(streamPath + '?' + query);
		source.addEventListener('ping', heard);
		source.addEventListener('clip', applyClip);
		source.addEventListener('playback', applyPlayback);
		source.addEventListener('preview', applyPreview);
		source.addEventListener('error', function () {
			// A stream the browser gave up on, such as one the auth guard
			// refused, is reopened by the next change to what the page shows.
			if (source && source.readyState === EventSource.CLOSED) {
				close();
			}
		});
	}

	function heard() {
		lastEventAt = Date.now();
	}

	function readUpdate(event) {
		heard();
		try {
			return JSON.parse(event.data);
		} catch (error) {
			console.warn('live stream sent an unreadable ' + event.type + ' update', error);

			return null;
		}
	}

	// morph swaps new markup into an element the way its own poll would, so
	// the page's swap handling wires it up as usual.
	function morph(target, html, style) {
		htmx.swap({ target: target, sourceElement: target, text: html, swap: style });
	}

	function applyClip(event) {
		var update = readUpdate(event);
		var region = update && document.getElementById('clip-' + update.id + '-status');
		if (region) {
			morph(region, update.html, 'outerMorph');
		}
	}

	function applyPlayback(event) {
		var update = readUpdate(event);
		if (!update) {
			return;
		}
		document.querySelectorAll('[data-playback-live]').forEach(function (panel) {
			if (panel.getAttribute('data-playback-live') === update.id) {
				morph(panel, update.html, 'innerMorph');
			}
		});
	}

	function applyPreview(event) {
		var update = readUpdate(event);
		if (update) {
			document.dispatchEvent(new CustomEvent('outtake:preview', { detail: update }));
		}
	}

	// delivering reports whether the stream follows something and has spoken
	// recently.
	function delivering(name, id) {
		return source !== null && followed[name][id] === true && Date.now() - lastEventAt < staleAfterMs;
	}

	// pollTarget names the feed an element's own poll belongs to.
	function pollTarget(element) {
		if (!element || !element.getAttribute) {
			return null;
		}
		var clip = element.getAttribute('data-clip-live');
		if (clip) {
			return { name: 'clip', id: clip };
		}
		var playback = element.getAttribute('data-playback-live');
		if (playback) {
			return { name: 'playback', id: playback };
		}

		return null;
	}

	window.outtakeLive = { delivering: delivering };

	document.addEventListener('htmx:before:request', function (event) {
		var target = pollTarget(event.target);
		if (target && delivering(target.name, target.id)) {
			event.preventDefault();
		}
	});
	document.addEventListener('htmx:after:swap', sync);
	document.addEventListener('outtake:live-sync', sync);
	document.addEventListener('visibilitychange', sync);
	window.addEventListener('pageshow', sync);
	window.addEventListener('pagehide', close);

	sync();
})();
