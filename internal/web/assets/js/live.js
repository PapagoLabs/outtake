// live.js keeps a page's rendering clip cards current over one event stream.
//
// Each rendering card's status region carries data-clip-live with its clip
// id. The stream sends a region whenever it changes, and a ping every second.
// While pings arrive, the regions' own polls are held back. When they stop,
// as behind a proxy that buffers the stream, the polls carry on.
//
// The stream holds one of the browser's few connections to the server, so it
// is open only while the page is visible and shows a rendering card.
(function () {
	'use strict';

	var streamPath = '/clips/live';
	// maxFollowed is the most clips one stream follows, as the server caps
	// it. Cards past it keep their own polls.
	var maxFollowed = 50;
	// staleAfterMs is how long after the last event the stream counts as
	// silent, which is two and a half of its one-second checks.
	var staleAfterMs = 2500;

	var source = null;
	var following = '';
	var followed = {};
	var lastEventAt = 0;

	// activeIds lists the clips whose cards are rendering, in a stable order,
	// up to maxFollowed.
	function activeIds() {
		var ids = [];
		document.querySelectorAll('[data-clip-live]').forEach(function (region) {
			ids.push(region.getAttribute('data-clip-live'));
		});

		return ids.sort().slice(0, maxFollowed);
	}

	function close() {
		if (source) {
			source.close();
			source = null;
		}
		following = '';
		followed = {};
		lastEventAt = 0;
	}

	// sync opens, reopens, or closes the stream to follow the rendering cards
	// the page shows now.
	function sync() {
		var list = document.hidden ? [] : activeIds();
		var ids = list.join(',');
		if (ids === following) {
			return;
		}
		close();
		if (!ids) {
			return;
		}
		following = ids;
		list.forEach(function (id) { followed[id] = true; });
		source = new EventSource(streamPath + '?ids=' + encodeURIComponent(ids));
		source.addEventListener('ping', heard);
		source.addEventListener('clip', applyUpdate);
		source.addEventListener('error', function () {
			// A stream the browser gave up on, such as one the auth guard
			// refused, is reopened by the next change to the cards.
			if (source && source.readyState === EventSource.CLOSED) {
				close();
			}
		});
	}

	function heard() {
		lastEventAt = Date.now();
	}

	// applyUpdate morphs a clip's new status region into its card, as the
	// region's own poll would.
	function applyUpdate(event) {
		heard();
		var update;
		try {
			update = JSON.parse(event.data);
		} catch (error) {
			console.warn('clip stream sent an unreadable update', error);
			return;
		}
		var region = document.getElementById('clip-' + update.id + '-status');
		if (!region) {
			return;
		}
		htmx.swap({ target: region, sourceElement: region, text: update.html, swap: 'outerMorph' });
	}

	// delivering reports whether the stream follows a clip and has spoken
	// recently.
	function delivering(id) {
		return source !== null && followed[id] === true && Date.now() - lastEventAt < staleAfterMs;
	}

	document.addEventListener('htmx:before:request', function (event) {
		var region = event.target;
		var id = region && region.getAttribute ? region.getAttribute('data-clip-live') : null;
		if (id && delivering(id)) {
			event.preventDefault();
		}
	});
	document.addEventListener('htmx:after:swap', sync);
	document.addEventListener('visibilitychange', sync);
	window.addEventListener('pageshow', sync);
	window.addEventListener('pagehide', close);

	sync();
})();
