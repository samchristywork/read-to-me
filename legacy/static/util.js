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
  let banner = document.createElement("div");
  banner.id='cookie-banner';
  banner.classList.add('cookie-banner');
  banner.innerHTML=`This website uses cookies to ensure you get the best experience and for authentication purposes.
    <a href="/privacy-policy" target="_blank">Learn More</a>
    <button onclick="acceptCookies()">Accept</button>`;
  document.body.appendChild(banner);
  if (document.cookie.indexOf("cookieConsent=true") === -1) {
    document.getElementById('cookie-banner').style.display = 'block';
  } else {
    document.getElementById('cookie-banner').style.display = 'none';
  }

  let username=getCookie("username");
  if (username) {
    document.querySelector("#profile").innerText="Hello, " + username;

    document.querySelector("#nav-login").style.display="none";
    document.querySelector("#nav-signup").style.display="none";
  } else {
    document.querySelector("#nav-logout").style.display="none";
    document.querySelector("#nav-user").style.display="none";
    document.querySelector("#nav-profile").style.display="none";
  }
}
