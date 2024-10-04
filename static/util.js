function getCookie(cname) {
  let name = cname + "=";
  let ca = document.cookie.split(';');
  for(let i = 0; i < ca.length; i++) {
    let c = ca[i];
    while (c.charAt(0) == ' ') {
      c = c.substring(1);
    }
    if (c.indexOf(name) == 0) {
      return c.substring(name.length, c.length);
    }
  }
  return "";
}

function acceptCookies() {
  document.cookie = "cookieConsent=true; path=/; max-age=" + 60 * 60 * 24 * 365;
  document.getElementById('cookie-banner').style.display = 'none';
}

window.onload = function() {
  if (document.cookie.indexOf("cookieConsent=true") === -1) {
    document.getElementById('cookie-banner').style.display = 'block';
  } else {
    document.getElementById('cookie-banner').style.display = 'none';
  }
}
