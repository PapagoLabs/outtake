(function () {
	var root = document.documentElement;
	var stored = window.localStorage.getItem('outtake-theme');
	if (stored === 'light') {
		root.classList.remove('dark');
	} else if (stored === 'dark') {
		root.classList.add('dark');
	}

	function syncThemeIcons() {
		var dark = root.classList.contains('dark');
		document.querySelectorAll('.js-theme-icon-dark').forEach(function (el) {
			el.classList.toggle('hidden', !dark);
		});
		document.querySelectorAll('.js-theme-icon-light').forEach(function (el) {
			el.classList.toggle('hidden', dark);
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

	document.addEventListener('htmx:afterSwap', bindExportForms);

	syncThemeIcons();
	bindExportForms();
})();
