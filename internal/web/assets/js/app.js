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
			el.classList.toggle('hidden', allowed.indexOf(type) === -1);
		});
	}

	function bindExportForms() {
		document.querySelectorAll('[data-export-form]').forEach(function (form) {
			applyExportForm(form);
			var select = form.querySelector('[name="clipType"]');
			if (select && !select.dataset.exportBound) {
				select.dataset.exportBound = '1';
				select.addEventListener('change', function () {
					applyExportForm(form);
				});
			}
		});
	}

	document.addEventListener('click', function (event) {
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

	document.addEventListener('htmx:after:swap', function (event) {
		bindExportForms();
		syncPaletteButtons();
		syncNav();
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
})();
