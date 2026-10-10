(function () {
	var root = document.documentElement;

	function syncThemeIcons() {
		var dark = root.classList.contains('dark');
		document.querySelectorAll('.js-theme-icon-dark').forEach(function (el) {
			el.classList.toggle('hidden', !dark);
		});
		document.querySelectorAll('.js-theme-icon-light').forEach(function (el) {
			el.classList.toggle('hidden', dark);
		});
	}

	function paletteList() {
		var raw = root.getAttribute('data-palettes');
		if (!raw) {
			return ['plex'];
		}

		return raw.split(',');
	}

	function syncPaletteButtons() {
		var current = root.getAttribute('data-palette') || 'plex';
		document.querySelectorAll('.js-palette').forEach(function (el) {
			var on = el.getAttribute('data-palette') === current;
			el.setAttribute('aria-pressed', on ? 'true' : 'false');
			el.classList.toggle('ring-2', on);
			el.classList.toggle('ring-sidebar-primary', on);
		});
	}

	function sidebarEl() {
		return document.querySelector('.js-sidebar');
	}

	function overlayEl() {
		return document.querySelector('.js-sidebar-overlay');
	}

	function openSidebar() {
		var sidebar = sidebarEl();
		var overlay = overlayEl();
		if (sidebar) {
			sidebar.classList.remove('-translate-x-full');
		}
		if (overlay) {
			overlay.classList.remove('hidden');
		}
	}

	function closeSidebar() {
		var sidebar = sidebarEl();
		var overlay = overlayEl();
		if (sidebar) {
			sidebar.classList.add('-translate-x-full');
		}
		if (overlay) {
			overlay.classList.add('hidden');
		}
	}

	var navOn = ['bg-sidebar-primary', 'text-sidebar-primary-foreground'];
	var navOff = ['text-sidebar-foreground', 'hover:bg-sidebar-accent', 'hover:text-sidebar-accent-foreground'];

	function setNavActive(el, on) {
		navOn.forEach(function (cls) {
			el.classList.toggle(cls, on);
		});
		navOff.forEach(function (cls) {
			el.classList.toggle(cls, !on);
		});
	}

	function syncNav() {
		var main = document.getElementById('main-content');
		var active = (main && main.getAttribute('data-nav')) || '';
		var library = (main && main.getAttribute('data-library')) || '';
		document.querySelectorAll('.js-sidebar [data-nav]').forEach(function (el) {
			setNavActive(el, active !== '' && el.getAttribute('data-nav') === active);
		});
		document.querySelectorAll('.js-sidebar [data-nav-library]').forEach(function (el) {
			setNavActive(el, library !== '' && el.getAttribute('data-nav-library') === library);
		});
	}

	function applyExportForm(form) {
		var select = form.querySelector('[name="clipType"]');
		var type = select ? select.value : 'clip';
		form.querySelectorAll('[data-export-for]').forEach(function (el) {
			var allowed = el.getAttribute('data-export-for').split(',');
			var hide = allowed.indexOf(type) === -1;
			el.classList.toggle('hidden', hide);
			el.toggleAttribute('hidden', hide);
		});
	}

	// formatDuration renders seconds in the short form used by the duration
	// readout, with empty parts left out. It mirrors media.Timecode.Short.
	function formatDuration(sec) {
		if (!isFinite(sec) || sec < 0) {
			sec = 0;
		}
		var whole = Math.round(sec);
		if (whole < 60) {
			return whole + 's';
		}
		var out = '';
		var h = Math.floor(whole / 3600);
		var m = Math.floor((whole % 3600) / 60);
		var s = whole % 60;
		if (h) {
			out += h + 'hr';
		}
		if (m) {
			out += m + 'min';
		}
		if (s) {
			out += s + 's';
		}
		return out;
	}

	function parseTimecode(value) {
		var parts = String(value).trim().split(':');
		var sec;
		if (parts.length === 1) {
			sec = parseFloat(parts[0]) || 0;
		} else if (parts.length === 2) {
			sec = (parseInt(parts[0], 10) || 0) * 60 + (parseFloat(parts[1]) || 0);
		} else {
			sec = (parseInt(parts[0], 10) || 0) * 3600 + (parseInt(parts[1], 10) || 0) * 60 + (parseFloat(parts[2]) || 0);
		}
		if (!isFinite(sec) || sec < 0) {
			return 0;
		}
		return sec;
	}

	function formControl(form, name) {
		return form.querySelector('[name="' + name + '"]');
	}

	// isSingleFrame reports whether the form is exporting a single frame.
	//
	// A screenshot is one frame at the start mark, so the end mark is hidden and
	// has no bearing on whether the selection is usable.
	function isSingleFrame(form) {
		var type = formControl(form, 'clipType');
		return !!type && type.value === 'screenshot';
	}

	// setSubmitsBlocked disables the export buttons and outlines them. A form
	// that is being submitted keeps its buttons disabled whatever the marks
	// say, so a later check cannot open the way to a second submission.
	//
	// The outline classes are carried on the button as data-invalid-css and are
	// appended rather than swapped in, so the button keeps its own variant and
	// shape. Appending also avoids a Tailwind conflict, since two border colours
	// in one class list resolve by stylesheet order, not attribute order.
	function setSubmitsBlocked(form, blocked) {
		var submitting = form.dataset.submitting === '1';
		form.querySelectorAll('.js-export-submit').forEach(function (btn) {
			if (!btn.dataset.okCss) {
				btn.dataset.okCss = btn.className;
			}
			btn.disabled = blocked || submitting;
			btn.className = blocked
				? btn.dataset.okCss + ' ' + btn.dataset.invalidCss
				: btn.dataset.okCss;
		});
	}

	// setFieldError marks a field invalid and shows its limit beside the label.
	//
	// The message is a fragment — "max 10min", "after 1hr" — rather than a
	// sentence, because it sits on the label's own line and a full sentence
	// wraps onto the row below.
	//
	// Only the first reason is shown. Several can apply at once and the field has
	// room for one; the order the caller passes them in is the order of
	// specificity, so the binding limit is the one reported.
	//
	// Every slot is written, not just the first. A mark has one label for a clip
	// and another for a screenshot, each carrying its own slot, and the two
	// containers are toggled by the export type. Writing only the first would
	// leave the other carrying a stale limit when the type changed.
	function setFieldError(form, name, invalid, message) {
		var el = formControl(form, name);
		if (el) {
			if (invalid) {
				el.setAttribute('aria-invalid', 'true');
			} else {
				el.removeAttribute('aria-invalid');
			}
		}
		var selector = '[data-' + name.replace(/([A-Z])/g, '-$1').toLowerCase() + '-error]';
		form.querySelectorAll(selector).forEach(function (slot) {
			if (invalid && message) {
				slot.textContent = message;
				slot.classList.remove('hidden');
			} else {
				// Cleared too, or a stale limit shows the moment the field is
				// corrected.
				slot.textContent = '';
				slot.classList.add('hidden');
			}
		});
	}

	// announce reads the full reasons for a screen reader.
	//
	// The visible text is deliberately terse, so this carries the whole
	// explanation for anyone not reading it by eye.
	function announce(form, reasons) {
		var live = form.querySelector('[data-duration-warning]');
		if (live) {
			live.textContent = reasons.join(' ');
		}
	}

	// syncExportDuration reports whether the selection is usable and shows the
	// result.
	//
	// It never rewrites a mark. Whatever the user typed is what they keep, so a
	// mistyped hour stays visible and correctable rather than being reset to
	// something they did not ask for. An out-of-bounds selection marks the
	// offending field, explains itself, and blocks the buttons that would encode
	// it.
	function syncExportDuration(form) {
		var startEl = formControl(form, 'startTime');
		var endEl = formControl(form, 'endTime');
		var durEl = formControl(form, 'duration');
		if (!startEl || !endEl || !durEl) {
			return;
		}
		var maxDur = parseInt(form.getAttribute('data-max-dur'), 10) || 600;
		// The source length, or zero when the page did not probe it. An 11 hour
		// start on a 2 hour film is a legal clip length, so only the media's own
		// duration catches a mark past the end of the source.
		var mediaDur = parseFloat(form.getAttribute('data-media-dur')) || 0;
		var warning = form.querySelector('[data-duration-warning]');

		var start = parseTimecode(startEl.value);
		var end = parseTimecode(endEl.value);
		var single = isSingleFrame(form);
		var dur = single ? 0 : Math.max(0, end - start);

		var startWhy = '';
		var endWhy = '';
		var reasons = [];

		// maxDur caps how long a clip may be, not how far into a film it may
		// start, so it is never applied to the start. Only the source's own length
		// bounds it, and an 11 hour start on a 2 hour film is a legal clip length.
		if (start < 0) {
			startWhy = 'min 0s';
			reasons.push('The start can\'t be negative.');
		} else if (mediaDur > 0 && start >= mediaDur) {
			startWhy = 'max ' + formatDuration(mediaDur);
			reasons.push('The start is past the end of the title, which is ' + formatDuration(mediaDur) + '.');
		}
		if (!single) {
			// The end's binding limit is whichever arrives first, the length cap or
			// the end of the source.
			var ceiling = mediaDur > 0 ? Math.min(start + maxDur, mediaDur) : start + maxDur;
			if (mediaDur > 0 && end > mediaDur) {
				endWhy = 'max ' + formatDuration(mediaDur);
				reasons.push('The end is past the end of the title, which is ' + formatDuration(mediaDur) + '.');
			} else if (dur > maxDur) {
				endWhy = 'max ' + formatDuration(maxDur);
				reasons.push('The selection is longer than the maximum of ' + formatDuration(maxDur) + '.');
			} else if (end <= start) {
				endWhy = 'after ' + formatDuration(start);
				reasons.push('The end must be after the start.');
			} else if (end > ceiling) {
				endWhy = 'max ' + formatDuration(ceiling);
				reasons.push('The end is past ' + formatDuration(ceiling) + '.');
			}
		}

		setFieldError(form, 'startTime', !!startWhy, startWhy);
		setFieldError(form, 'endTime', !!endWhy, endWhy);
		setSubmitsBlocked(form, reasons.length > 0);
		announce(form, reasons);

		durEl.value = dur.toFixed(3);
		var label = form.querySelector('[data-duration-label]');
		if (label) {
			label.textContent = formatDuration(dur);
		}
	}

	// screenShowsHDR reports whether the browser says its screen shows HDR
	// video. Firefox only answers the video-dynamic-range query.
	function screenShowsHDR() {
		return window.matchMedia('(video-dynamic-range: high)').matches ||
			window.matchMedia('(dynamic-range: high)').matches;
	}

	// playsHEVC reports whether the browser can decode the HEVC Main 10 an HDR
	// preview is encoded in. Chrome on Linux without hardware decoding cannot.
	function playsHEVC() {
		var probe = document.createElement('video');
		return probe.canPlayType('video/mp4; codecs="hvc1.2.4.L93.B0"') !== '';
	}

	// markScreen tells the server whether this browser can show an HDR
	// preview, so a preview of an HDR clip is tone mapped wherever its HDR
	// screen or its HEVC decoder is missing.
	function markScreen(form) {
		var field = form.querySelector('[data-screen-hdr]');
		if (field) {
			field.value = screenShowsHDR() && playsHEVC() ? '1' : '0';
		}
	}

	// clipFileURL builds the URL of a clip's own file from its id and version,
	// on the fixed /clips path, so no URL is read back from the page.
	function clipFileURL(id, version) {
		return '/clips/' + encodeURIComponent(id) + '/file?v=' + encodeURIComponent(version);
	}

	// playerFrame is the element a note about a player goes beside: the frame
	// that holds the player and its badge, or the player itself without one.
	function playerFrame(video) {
		return video.closest('[data-player-frame]') || video;
	}

	// showFormatBadge shows the badge naming the file a clip's player plays.
	// Both badges are rendered by the server in the player's frame, so only
	// their visibility changes.
	function showFormatBadge(video, which) {
		playerFrame(video).querySelectorAll('[data-format-badge]').forEach(function (badge) {
			badge.hidden = badge.getAttribute('data-format-badge') !== which;
		});
	}

	// chooseClipSources moves each HDR clip's player from its SDR version to
	// the HDR file where the screen shows HDR and the browser decodes HEVC, the
	// same test the export form applies to previews. A clip with no SDR version
	// says so where its HDR file cannot be shown. Each player is visited once.
	function chooseClipSources() {
		var showsHDR = screenShowsHDR() && playsHEVC();
		document.querySelectorAll('video[data-hdr-version]').forEach(function (video) {
			if (video.dataset.sourceChosen) {
				return;
			}
			video.dataset.sourceChosen = '1';
			if (showsHDR && video.dataset.clipId) {
				video.src = clipFileURL(video.dataset.clipId, video.dataset.hdrVersion);
				showFormatBadge(video, 'hdr');
			}
		});
		document.querySelectorAll('video[data-sdr-missing]').forEach(function (video) {
			if (video.dataset.sourceChosen) {
				return;
			}
			video.dataset.sourceChosen = '1';
			if (showsHDR) {
				return;
			}
			var note = document.createElement('p');
			note.setAttribute('data-sdr-missing-note', '');
			note.className = 'mb-2 text-xs text-muted-foreground';
			note.textContent = 'Regenerate this clip to make a preview this screen can play';
			playerFrame(video).insertAdjacentElement('beforebegin', note);
		});
	}

	// explainPlaybackError says why a clip's player stays empty when the
	// browser cannot decode the file, such as HEVC in Chrome on Linux. The
	// file itself is fine, so the note points at the download.
	function explainPlaybackError(video) {
		if (video.nextElementSibling && video.nextElementSibling.hasAttribute('data-playback-error')) {
			return;
		}
		var note = document.createElement('p');
		note.setAttribute('data-playback-error', '');
		note.setAttribute('role', 'status');
		note.className = 'mt-2 text-sm text-destructive';
		note.textContent = 'This browser can\'t play this clip. Download it to watch it.';
		video.insertAdjacentElement('afterend', note);
	}

	// holdSubmits marks a form as being submitted and disables every export
	// button on it, not just the one clicked, so neither can send it again.
	function holdSubmits(form) {
		form.dataset.submitting = '1';
		form.querySelectorAll('.js-export-submit').forEach(function (btn) {
			btn.disabled = true;
		});
	}

	// releaseSubmits ends a submission's hold and checks the marks again.
	function releaseSubmits(form) {
		if (form.dataset.submitting !== '1') {
			return;
		}
		delete form.dataset.submitting;
		syncExportDuration(form);
	}

	// bindExportForms sets up each export form once. A swap elsewhere on the
	// page, such as the Plex position refreshing, leaves forms already set up
	// as they are, so it cannot undo a submission in flight.
	function bindExportForms() {
		document.querySelectorAll('[data-export-form]').forEach(function (form) {
			if (form.dataset.exportReady) {
				return;
			}
			form.dataset.exportReady = '1';
			applyExportForm(form);
			markScreen(form);
			syncExportDuration(form);
			var select = form.querySelector('[name="clipType"]');
			if (select && !select.dataset.exportBound) {
				select.dataset.exportBound = '1';
				select.addEventListener('change', function () {
					applyExportForm(form);
					// The type decides which marks matter. A screenshot hides the
					// end mark and is bounded by its start alone, so the bounds and
					// the button state have to be worked out again after the switch
					// rather than left over from the previous type.
					syncExportDuration(form);
				});
			}
			if (!form.dataset.durationBound) {
				form.dataset.durationBound = '1';
				var startEl = formControl(form, 'startTime');
				var endEl = formControl(form, 'endTime');
				if (startEl) {
					startEl.addEventListener('input', function () {
						syncExportDuration(form);
					});
				}
				if (endEl) {
					endEl.addEventListener('input', function () {
						syncExportDuration(form);
					});
				}
			}
		});
	}

	function jumpSelector(key) {
		if (window.CSS && CSS.escape) {
			return '[data-jump="' + CSS.escape(key) + '"]';
		}

		return '[data-jump="' + String(key).replace(/\\/g, '\\\\').replace(/"/g, '\\"') + '"]';
	}

	function parseJumpYear(key) {
		var y = parseInt(key, 10);
		if (isFinite(y) && String(y) === key) {
			return y;
		}
		if (key && key.length >= 7 && key.charAt(2) === '/') {
			y = parseInt(key.slice(3), 10);
			var m = parseInt(key.slice(0, 2), 10);
			if (isFinite(y) && isFinite(m)) {
				return y * 12 + m;
			}
		}
		return NaN;
	}

	function coveringRailKey(current) {
		var nav = document.querySelector('[data-jump-nav]');
		var nodes = document.querySelectorAll('[data-jump-key]');
		var keys = [];
		nodes.forEach(function (el) {
			keys.push(el.getAttribute('data-jump-key'));
		});
		if (!current || keys.length === 0) {
			return current;
		}
		if (keys.indexOf(current) !== -1) {
			return current;
		}
		var kind = nav ? nav.getAttribute('data-jump-kind') : '';
		if (kind === 'year' || kind === 'month') {
			var cur = parseJumpYear(current);
			if (!isFinite(cur)) {
				return keys[0];
			}
			var first = parseJumpYear(keys[0]);
			var last = parseJumpYear(keys[keys.length - 1]);
			var desc = isFinite(first) && isFinite(last) && first > last;
			var best = keys[0];
			for (var i = 0; i < keys.length; i++) {
				var val = parseJumpYear(keys[i]);
				if (!isFinite(val)) {
					continue;
				}
				if (desc) {
					if (val >= cur) {
						best = keys[i];
					} else {
						break;
					}
				} else if (val <= cur) {
					best = keys[i];
				} else {
					break;
				}
			}
			return best;
		}
		var bestLetter = keys[0];
		for (var j = 0; j < keys.length; j++) {
			if (keys[j] <= current || keys[j] === '#') {
				bestLetter = keys[j];
			} else {
				break;
			}
		}
		return bestLetter;
	}

	function highlightJump(key) {
		if (!key) {
			return;
		}
		var active = coveringRailKey(key);
		document.querySelectorAll('[data-jump-key]').forEach(function (el) {
			var on = el.getAttribute('data-jump-key') === active;
			el.classList.toggle('bg-primary', on);
			el.classList.toggle('text-primary-foreground', on);
			el.classList.toggle('text-muted-foreground', !on);
			if (on) {
				el.setAttribute('aria-current', 'true');
			} else {
				el.removeAttribute('aria-current');
			}
		});
		var select = document.getElementById('media-list-letter');
		if (select && active) {
			select.value = active;
		}
	}

	var jumpTicking = false;

	function spyMediaJump() {
		jumpTicking = false;
		var results = document.getElementById('media-results');
		if (!results) {
			return;
		}
		var rootTop = results.getBoundingClientRect().top;
		var active = '';
		var cards = results.querySelectorAll('[data-jump]');
		for (var i = 0; i < cards.length; i++) {
			var card = cards[i];
			if (card.getBoundingClientRect().top > rootTop + 96) {
				break;
			}
			active = card.getAttribute('data-jump');
		}
		if (!active && cards.length) {
			active = cards[0].getAttribute('data-jump');
		}
		if (active) {
			highlightJump(active);
		}
	}

	function pinPreviousSentinel() {
		var results = document.getElementById('media-results');
		var prev = document.getElementById('media-prev');
		if (!results || !prev || results.scrollTop > 0) {
			return;
		}
		var first = results.querySelector('[data-jump]');
		if (!first) {
			return;
		}
		var top = first.getBoundingClientRect().top - results.getBoundingClientRect().top + results.scrollTop;
		if (top > 0) {
			results.scrollTop = top;
		}
	}

	function bindMediaJump() {
		pinPreviousSentinel();
		spyMediaJump();
	}

	document.addEventListener('scroll', function (event) {
		if (!event.target || event.target.id !== 'media-results') {
			return;
		}
		if (jumpTicking) {
			return;
		}
		jumpTicking = true;
		window.requestAnimationFrame(spyMediaJump);
	}, { capture: true, passive: true });

	document.addEventListener('click', function (event) {
		var jump = event.target.closest('.js-media-jump');
		if (jump) {
			var results = document.getElementById('media-results');
			var key = jump.getAttribute('data-jump-key');
			var target = results && key ? results.querySelector(jumpSelector(key)) : null;
			if (target) {
				event.preventDefault();
				event.stopImmediatePropagation();
				target.scrollIntoView({ block: 'start' });
				highlightJump(key);
				if (jump.href) {
					window.history.replaceState({}, '', jump.href);
				}
				return;
			}
		}
		if (event.target.closest('.js-theme-toggle')) {
			var dark = root.classList.toggle('dark');
			window.localStorage.setItem('outtake-theme', dark ? 'dark' : 'light');
			syncThemeIcons();
		}
		var paletteBtn = event.target.closest('.js-palette');
		if (paletteBtn) {
			var palette = paletteBtn.getAttribute('data-palette');
			if (palette && paletteList().indexOf(palette) !== -1) {
				root.setAttribute('data-palette', palette);
				window.localStorage.setItem('outtake-palette', palette);
				syncPaletteButtons();
			}
		}
		if (event.target.closest('.js-sidebar-open')) {
			openSidebar();
		}
		if (event.target.closest('.js-sidebar-close') || event.target.closest('.js-sidebar-overlay')) {
			closeSidebar();
		}
		var dismiss = event.target.closest('.js-flash-dismiss');
		if (dismiss) {
			var flash = dismiss.closest('.js-flash');
			if (flash) {
				flash.remove();
			}
			var url = new URL(window.location.href);
			if (url.searchParams.has('error')) {
				url.searchParams.delete('error');
				window.history.replaceState({}, '', url);
			}
		}
	});

	// A media element's error does not bubble, so it is caught on the way down.
	document.addEventListener('error', function (event) {
		var target = event.target;
		if (target instanceof HTMLVideoElement && target.hasAttribute('data-clip-video')) {
			explainPlaybackError(target);
		}
	}, true);

	document.addEventListener('submit', function (event) {
		var form = event.target;
		if (!(form instanceof HTMLFormElement)) {
			return;
		}
		var message = form.getAttribute('data-confirm');
		if (message && !window.confirm(message)) {
			event.preventDefault();
			return;
		}
		var btn = event.submitter;
		if (btn && btn.classList.contains('js-export-submit')) {
			holdSubmits(form);
		}
	});

	// An htmx submission ends without leaving the page, so its form is
	// released when the request finishes, whatever the answer was.
	document.addEventListener('htmx:finally:request', function (event) {
		var elt = event.target;
		var form = elt && elt.closest ? elt.closest('form') : null;
		if (form) {
			releaseSubmits(form);
		}
	});

	// A page restored from the back-forward cache comes back with the hold of
	// the submission that left it, so it is released.
	window.addEventListener('pageshow', function () {
		document.querySelectorAll('[data-export-form]').forEach(releaseSubmits);
	});

	document.addEventListener('htmx:before:swap', function (event) {
		var results = document.getElementById('media-results');
		var detail = event.detail || {};
		var src = detail.sourceElement;
		if (!src && detail.ctx) {
			src = detail.ctx.sourceElement;
		}
		if (!results || !src || src.id !== 'media-prev') {
			return;
		}
		results.dataset.prevScrollHeight = String(results.scrollHeight);
		results.dataset.prevScrollTop = String(results.scrollTop);
	});

	document.addEventListener('htmx:after:swap', function (event) {
		bindExportForms();
		chooseClipSources();
		syncPaletteButtons();
		syncNav();
		bindMediaJump();
		var results = document.getElementById('media-results');
		if (results && results.dataset.prevScrollHeight != null) {
			var prevH = parseInt(results.dataset.prevScrollHeight, 10);
			var prevTop = parseInt(results.dataset.prevScrollTop, 10);
			delete results.dataset.prevScrollHeight;
			delete results.dataset.prevScrollTop;
			if (isFinite(prevH) && isFinite(prevTop)) {
				results.scrollTop = prevTop + (results.scrollHeight - prevH);
			}
		}
		var detail = event.detail || {};
		var target = detail.target;
		if (!target && detail.ctx) {
			target = detail.ctx.target;
		}
		if (target && target.id === 'main-content') {
			closeSidebar();
		}
	});

	syncThemeIcons();
	syncPaletteButtons();
	syncNav();
	bindExportForms();
	chooseClipSources();
	bindMediaJump();
})();
