(function () {
	var startEl = document.getElementById('startTime');
	var endEl = document.getElementById('endTime');
	var durEl = document.getElementById('duration');
	if (!startEl || !endEl || !durEl) {
		return;
	}
	function pad2(n) { return String(n).padStart(2, '0'); }

	function formatTimecode(sec) {
		if (!isFinite(sec) || sec < 0) { sec = 0; }
		var ms = Math.round(sec * 1000);
		var whole = Math.floor(ms / 1000);
		var frac = ms % 1000;
		var h = Math.floor(whole / 3600);
		var m = Math.floor((whole % 3600) / 60);
		var s = whole % 60;
		return pad2(h) + ':' + pad2(m) + ':' + pad2(s) + '.' + String(frac).padStart(3, '0');
	}
	function parseTimecode(value) {
		var parts = String(value).trim().split(':');
		if (parts.length === 1) { return parseFloat(parts[0]) || 0; }
		if (parts.length === 2) { return (parseInt(parts[0], 10) || 0) * 60 + (parseFloat(parts[1]) || 0); }
		return (parseInt(parts[0], 10) || 0) * 3600 + (parseInt(parts[1], 10) || 0) * 60 + (parseFloat(parts[2]) || 0);
	}
	// The bounds themselves live in app.js, which already binds every
	// [data-export-form] including this one. This file only moves the marks; it
	// never validates or rewrites them, so there is only one implementation and
	// the two cannot disagree.
	//
	// requestSync asks the shared binder to re-validate the form.
	function requestSync() {
		startEl.dispatchEvent(new Event('input', { bubbles: true }));
	}

	// markStart moves the start mark to an offset. When that leaves the end at
	// or before it, the end is carried along so the range keeps its length.
	// The shared binder then reports any limit the new range breaks, so a
	// carried end never goes unexplained.
	function markStart(offset) {
		startEl.value = formatTimecode(offset);
		var start = parseTimecode(startEl.value);
		var end = parseTimecode(endEl.value);
		if (end <= start) {
			var dur = parseFloat(durEl.value) || 0;
			if (dur <= 0) { dur = 10; }
			endEl.value = formatTimecode(start + dur);
		}
		requestSync();
	}

	function markEnd(offset) {
		endEl.value = formatTimecode(offset);
		requestSync();
	}

	// positionTimeoutMs gives up on a fresh read a little after the server
	// gives up on Plex, so the panel's position is used instead.
	var positionTimeoutMs = 4000;

	// readPosition asks the server where Plex is now. The panel refreshes on
	// its own schedule, and a client that is playing moves on in between, so
	// the position is read at the moment of the click. The panel's position is
	// the fallback when Plex cannot be asked or no longer plays the item.
	function readPosition(btn) {
		var shown = parseFloat(btn.getAttribute('data-offset'));
		var url = btn.getAttribute('data-position-url');
		if (!url || typeof AbortController === 'undefined') {
			return Promise.resolve(shown);
		}
		var controller = new AbortController();
		var timer = window.setTimeout(function () { controller.abort(); }, positionTimeoutMs);
		return fetch(url, { headers: { 'Accept': 'application/json' }, signal: controller.signal })
			.then(function (res) {
				if (!res.ok) { return null; }
				return res.json();
			})
			.then(function (position) {
				return position && position.playing ? position.offset : shown;
			})
			.catch(function () {
				return shown;
			})
			.finally(function () {
				window.clearTimeout(timer);
			});
	}

	// latestRead numbers each mark's position reads. The panel is swapped every
	// couple of seconds, and the fresh button can start a second read while
	// the first is still out, so only the newest read for a mark is applied.
	var latestRead = { start: 0, end: 0 };

	document.addEventListener('click', function (event) {
		var btn = event.target.closest('.js-mark-start, .js-mark-end');
		if (!btn || btn.disabled) {
			return;
		}
		var which = btn.classList.contains('js-mark-start') ? 'start' : 'end';
		var mark = which === 'start' ? markStart : markEnd;
		var read = ++latestRead[which];
		btn.disabled = true;
		readPosition(btn).then(function (offset) {
			btn.disabled = false;
			if (read === latestRead[which]) {
				mark(offset);
			}
		});
	});
	requestSync();

	// The preview renders in the background, so the file this page was
	// redirected to may not exist yet. Poll until the server publishes it, then
	// hand the URL to the player rather than pointing at it up front.
	var videoEl = document.getElementById('preview-video');
	var statusEl = document.getElementById('preview-status');
	if (!videoEl || !statusEl) { return; }

	var progressEl = document.getElementById('preview-progress');
	var cancelEl = document.getElementById('preview-cancel');
	var buttonEl = document.getElementById('preview-button');
	var buttonLabelEl = document.getElementById('preview-button-label');
	var buttonIdleLabel = buttonLabelEl ? buttonLabelEl.textContent : 'Preview';

	var previewId = videoEl.getAttribute('data-preview-id');
	var pollIntervalMs = 1000;
	var retryIntervalMs = 2000;
	// 900 attempts at the poll interval is about fifteen minutes, which is far
	// longer than any encode this page waits on, so reaching it means the
	// preview is never going to arrive.
	var maxAttempts = 900;
	var attempts = 0;
	var stopped = false;
	var pendingTimer = null;

	// htmx names its teardown events "htmx:before:cleanup" and
	// "htmx:after:cleanup". Only a cleanup that reaches this player stops the
	// poll: the page also swaps a Plex position fragment every couple of
	// seconds, and that must not end the preview poll.
	function coversPreview(node) {
		return node === videoEl || (node && typeof node.contains === 'function' && node.contains(videoEl));
	}

	function stopPolling() {
		stopped = true;
		if (pendingTimer !== null) {
			window.clearTimeout(pendingTimer);
			pendingTimer = null;
		}
	}

	document.addEventListener('htmx:before:cleanup', function (event) {
		var node = (event.detail && (event.detail.elt || event.detail.eltId)) || event.target;
		if (coversPreview(node)) { stopPolling(); }
	});

	function exhausted() {
		return attempts >= maxAttempts;
	}

	function schedule(delay) {
		if (stopped || exhausted()) { return; }
		pendingTimer = window.setTimeout(pollPreview, delay);
	}

	// showProgress marks a render as underway: the bar appears and advances, a
	// cancel control is offered, and the submit button is held down so a second
	// click reads as "already rendering" rather than looking like it did nothing.
	//
	// Only aria-valuenow is set. The progress component's own script watches that
	// attribute and drives the bar width, so writing style here would fight it.
	function showProgress(percent) {
		var value = Math.max(0, Math.min(100, percent || 0));

		if (progressEl) {
			progressEl.classList.remove('hidden');
			progressEl.setAttribute('aria-valuenow', String(value));
		}
		if (cancelEl) {
			cancelEl.classList.remove('hidden');
		}
		if (buttonEl) {
			buttonEl.disabled = true;
			buttonEl.classList.add('opacity-60');
		}
		if (buttonLabelEl) {
			buttonLabelEl.textContent = 'Rendering…';
		}
	}

	// clearProgress hides the in-flight affordances and restores the button.
	function clearProgress() {
		if (progressEl) { progressEl.classList.add('hidden'); }
		if (cancelEl) { cancelEl.classList.add('hidden'); }
		if (buttonEl) {
			buttonEl.disabled = false;
			buttonEl.classList.remove('opacity-60');
		}
		if (buttonLabelEl) { buttonLabelEl.textContent = buttonIdleLabel; }
	}

	function giveUp(message) {
		stopPolling();
		clearProgress();
		statusEl.classList.remove('hidden');
		statusEl.textContent = message;
	}

	if (cancelEl) {
		// The cancel itself is an htmx request, so it inherits the CSRF token the
		// layout sets on the body. Polling continues either way: the render ends
		// as canceled and the next poll reports it, so the status line settles on
		// the outcome rather than freezing at the last percentage.
		cancelEl.addEventListener('htmx:response:error', function () {
			statusEl.classList.remove('hidden');
			statusEl.textContent = 'Could not cancel the preview.';
		});
	}

	// The proxy is a straight -ss/-t with no speed change, so a position in it
	// is the same source time. Picking the end mark against the loaded footage
	// is therefore exact, and Preview then renders the selection as typed.
	var setEndEl = document.getElementById('preview-set-end');
	var proxyStart = parseFloat(videoEl.getAttribute('data-preview-start')) || 0;

	if (setEndEl) {
		// Revealed once the player can be seeked, which is when a current position
		// means anything. The poll can report the preview as already finished
		// before the element ever loads, so waiting on the poll alone would leave
		// the control hidden.
		videoEl.addEventListener('loadedmetadata', function () {
			setEndEl.classList.remove('hidden');
		});

		setEndEl.addEventListener('click', function () {
			if (!endEl || !Number.isFinite(videoEl.currentTime)) { return; }
			endEl.value = formatTimecode(proxyStart + videoEl.currentTime);
			endEl.dispatchEvent(new Event('input'));
		});
	}

	function pollPreview() {
		// A response can land after the page moved on, so the player is checked
		// before it is touched.
		if (stopped || !document.contains(videoEl)) {
			stopPolling();

			return;
		}

		fetch('/api/clips/preview/' + encodeURIComponent(previewId))
			.then(function (response) {
				if (response.status === 404) {
					// The id is not registered, so polling it will never succeed.
					giveUp('Preview is no longer available.');

					return null;
				}
				if (!response.ok) {
					throw new Error('status ' + response.status);
				}

				return response.json();
			})
			.then(function (state) {
				if (state === null || stopped || !document.contains(videoEl)) { return; }
				if (state.status === 'completed' && state.url) {
					stopPolling();
					clearProgress();
					statusEl.textContent = '';
					statusEl.classList.add('hidden');
					videoEl.src = state.url;
					videoEl.load();

					return;
				}
				if (state.status === 'failed' || state.status === 'canceled') {
					stopPolling();
					clearProgress();
					statusEl.classList.remove('hidden');
					statusEl.textContent = state.error
						? state.status.charAt(0).toUpperCase() + state.status.slice(1) + ': ' + state.error
						: 'Preview was ' + state.status + '.';

					return;
				}
				attempts++;
				if (exhausted()) {
					giveUp('Gave up waiting for the preview after ' + maxAttempts + ' attempts.');

					return;
				}
				showProgress(state.progress);
				statusEl.textContent = state.progress
					? 'Rendering preview… ' + state.progress + '%'
					: 'Rendering preview…';
				schedule(pollIntervalMs);
			})
			.catch(function (error) {
				if (stopped || !document.contains(videoEl)) { return; }
				console.warn('preview poll failed for ' + previewId, error);
				attempts++;
				if (exhausted()) {
					giveUp('Lost contact with the preview server.');

					return;
				}
				showProgress(0);
				statusEl.textContent = 'Waiting for the preview server…';
				schedule(retryIntervalMs);
			});
	}

	pollPreview();
})();
