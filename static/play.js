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

  shas = data['shas'];
  sessionID = data['sessionID'];
  currentIndex = 0;

  audios = shas.map(sha => new Audio("data/output-"+sha+".mp3"));

  let requests = shas.map(filename =>
    fetch("data/text-"+filename+".txt")
    .then(response => {
      if (response.ok) {
        return response.text();
      }
    })
  );

  Promise.all(requests).then((values) => {
    for(value of values) {
      console.log(value)
      var newDiv = document.createElement('div');
      newDiv.textContent = value;
      document.getElementById('scrollable').appendChild(newDiv);
      console.log("Added");

      var newDiv = document.createElement('div');
      newDiv.textContent = value;
      document.getElementById('sections').appendChild(newDiv);
    }
  });

  var loaded = 0;
  var duration = 0;
  audios.forEach(audio => {
    audio.onloadeddata = function() {
      loaded++;
      duration += audio.duration;
      if (loaded == audios.length) {
        document.getElementById('totalDuration').innerText = formatDuration(duration);
        //playAudioSequence();
      }
    };
  });
});
