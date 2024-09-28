function setCookie(cname, cvalue, exdays) {
  const d = new Date();
  d.setTime(d.getTime() + (exdays * 24 * 60 * 60 * 1000));
  let expires = "expires="+d.toUTCString();
  document.cookie = cname + "=" + cvalue + ";SameSite=strict; Secure; " + expires + ";path=/";
}

function login() {
  fetch('/login', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({
      username: document.querySelector("#username").value,
      password: document.querySelector("#password").value
    })
  }).then(function(response) {
    return response.json();
  }).then(function(data) {
    if(data.status=="ok") {
      setCookie("username", data.username, 30);
      setCookie("token", data.token, 30);
      window.location.href="/";
    } else {
      setCookie("username", "", 30);
      document.querySelector(".error").innerText="Login Failed";
    }
  });
}
