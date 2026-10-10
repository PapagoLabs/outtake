// The site has only the Plex dark scheme, so every theme request, including
// one stored by an earlier visit, applies dark.

function setTheme() {
  document.documentElement.classList.remove("light");
  document.documentElement.classList.add("dark");
  document.documentElement.style.colorScheme = "dark";
}

setTheme();
