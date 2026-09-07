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

	function pad2(n) {
		return String(n).padStart(2, '0');
	}

	function formatTimecode(sec) {
		if (!isFinite(sec) || sec < 0) {
			sec = 0;
		}
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

	function syncExportDuration(form) {
		var startEl = formControl(form, 'startTime');
		var endEl = formControl(form, 'endTime');
		var durEl = formControl(form, 'duration');
		if (!startEl || !endEl || !durEl) {
			return;
		}
		var maxDur = parseInt(form.getAttribute('data-max-dur'), 10) || 600;
		var start = parseTimecode(startEl.value);
		var end = parseTimecode(endEl.value);
		var dur = Math.max(0, end - start);
		var warning = form.querySelector('[data-duration-warning]');
		if (dur > maxDur) {
			dur = maxDur;
			if (warning) {
				warning.classList.remove('hidden');
			}
		} else if (warning) {
			warning.classList.add('hidden');
		}
		durEl.value = dur.toFixed(3);
		var label = form.querySelector('[data-duration-label]');
		if (label) {
			label.textContent = formatTimecode(dur);
		}
	}

	function bindExportForms() {
		document.querySelectorAll('[data-export-form]').forEach(function (form) {
			applyExportForm(form);
			syncExportDuration(form);
			var select = form.querySelector('[name="clipType"]');
			if (select && !select.dataset.exportBound) {
				select.dataset.exportBound = '1';
				select.addEventListener('change', function () {
					applyExportForm(form);
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
			btn.setAttribute('disabled', 'disabled');
		}
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
	bindMediaJump();
})();
