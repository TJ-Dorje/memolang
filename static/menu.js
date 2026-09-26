// Progressive enhancement for the nav's account menu. <details> already opens
// and closes on its own; this only closes it on a click elsewhere or Escape,
// which the element does not do natively. Without JS the menu still works.
document.addEventListener("click", (e) => {
    document.querySelectorAll("details.account-menu[open]").forEach((menu) => {
        if (!menu.contains(e.target)) menu.open = false;
    });
});
document.addEventListener("keydown", (e) => {
    if (e.key !== "Escape") return;
    document.querySelectorAll("details.account-menu[open]").forEach((menu) => {
        menu.open = false;
        menu.querySelector("summary").focus();
    });
});
