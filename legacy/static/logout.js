function setCookie(cname, cvalue, exdays) {
  const d = new Date();
  d.setTime(d.getTime() + (exdays * 24 * 60 * 60 * 1000));
  let expires = "expires="+d.toUTCString();
  document.cookie = cname + "=" + cvalue + ";SameSite=strict;" + expires + ";path=/";
}

function logout() {
  setCookie("username", "", 30);
  setCookie("token", "", 30);
  window.location="/";
}
