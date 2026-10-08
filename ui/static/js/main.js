var navLinks = document.querySelectorAll("nav a");
for (var i = 0; i < navLinks.length; i++) {
	var link = navLinks[i]
	if (link.getAttribute('href') == window.location.pathname) {
		link.classList.add("live");
		break;
	}
}
(() => {
  const picker = document.querySelector('.star-picker');
  if (!picker) return;
  const output = document.querySelector('.selected-rating');
  picker.addEventListener('change', event => {
    if (event.target.name === 'rating') output.textContent = `${event.target.value} / 5 stars`;
  });
})();
