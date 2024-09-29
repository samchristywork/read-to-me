var texts = [];
var audios = [];

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

function formatDuration(duration) {
  var minutes = Math.floor(duration / 60);
  var seconds = Math.floor(duration % 60);
  return minutes + "m " + seconds + "s";
}

let token=getCookie("token")

fetch('/play', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json'
  },
  body: JSON.stringify({
    session: "SESSION TOKEN",
    token: token,
  })
}).then(function(response) {
  return response.json();
}).then(function(data) {
  console.log(data);
  if (data.status != "ok") {
    return;
  }
});
