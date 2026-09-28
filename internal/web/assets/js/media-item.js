(function () {
	var startEl = document.getElementById('startTime');
	var endEl = document.getElementById('endTime');
	var durEl = document.getElementById('duration');
	var label = document.getElementById('duration-label');
	var warning = document.getElementById('duration-warning');
	var maxDur = parseInt(document.getElementById('clip-form-config').getAttribute('data-max-dur'), 10) || 600;
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
	function syncDuration() {
		var start = parseTimecode(startEl.value);
		var end = parseTimecode(endEl.value);
		var dur = Math.max(0, end - start);
		if (dur > maxDur) {
			dur = maxDur;
			if (warning) { warning.classList.remove('hidden'); }
		} else if (warning) {
			warning.classList.add('hidden');
		}
		durEl.value = dur.toFixed(3);
		label.textContent = formatTimecode(dur);
	}
	document.addEventListener('click', function (event) {
		var startBtn = event.target.closest('.js-mark-start');
		var endBtn = event.target.closest('.js-mark-end');
		if (startBtn) {
			startEl.value = formatTimecode(parseFloat(startBtn.getAttribute('data-offset')));
			var start = parseTimecode(startEl.value);
			var end = parseTimecode(endEl.value);
			if (end <= start) {
				var dur = parseFloat(durEl.value) || 0;
				if (dur <= 0) { dur = 10; }
				endEl.value = formatTimecode(start + dur);
			}
			syncDuration();
		}
		if (endBtn) {
			endEl.value = formatTimecode(parseFloat(endBtn.getAttribute('data-offset')));
			syncDuration();
		}
	});
	startEl.addEventListener('input', syncDuration);
	endEl.addEventListener('input', syncDuration);
	syncDuration();

	// The preview renders in the background, so the file this page was
	// redirected to may not exist yet. Poll until the server publishes it, then
	// hand the URL to the player rather than pointing at it up front.
	var videoEl = document.getElementById('preview-video');
	var statusEl = document.getElementById('preview-status');
	if (!videoEl || !statusEl) { return; }

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

	function giveUp(message) {
		stopped = true;
		statusEl.classList.remove('hidden');
		statusEl.textContent = message;
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
					statusEl.textContent = '';
					statusEl.classList.add('hidden');
					videoEl.src = state.url;
					videoEl.load();

					return;
				}
				if (state.status === 'failed' || state.status === 'canceled') {
					stopPolling();
					statusEl.textContent = state.error || 'Preview was not rendered.';

					return;
				}
				attempts++;
				if (exhausted()) {
					giveUp('Gave up waiting for the preview after ' + maxAttempts + ' attempts.');

					return;
				}
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
				statusEl.textContent = 'Waiting for the preview server…';
				schedule(retryIntervalMs);
			});
	}

	pollPreview();
})();
