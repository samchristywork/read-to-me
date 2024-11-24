var fragments = [];
var currentIndex = 0;
var currentAudio = null;
var currentSpeed = 1.0;

const currentUrl = window.location.href;
const url = new URL(currentUrl);
const searchParams = url.searchParams;
const sParameter = searchParams.get('s');
console.log(sParameter);

function nextAudio() {
  console.log('nextAudio');

  if (currentAudio) {
    currentAudio.pause();
    currentAudio.remove();
  }
  currentIndex++;
  if (currentIndex < fragments.length) {
    playAudioSequence();
  }
}

function prevAudio() {
  console.log('prevAudio');

  if (currentAudio) {
    currentAudio.pause();
    currentAudio.remove();
  }
  currentIndex--;
  if (currentIndex >= 0) {
    playAudioSequence();
  }
}

function playPauseAudio() {
  console.log('playPauseAudio');

  if (currentAudio) {
    if (currentAudio.paused) {
      currentAudio.play();
    } else {
      currentAudio.pause();
    }
  }
}

function restartAudio() {
  console.log('restartAudio');

  currentIndex = 0;
  if (currentAudio) {
    currentAudio.pause();
    currentAudio.remove();
  }
  playAudioSequence();
}

function setAudioSpeed(speed) {
  console.log('setAudioSpeed');

  document.getElementById('speedValue').innerText = speed;
  currentSpeed = speed;
  if (currentAudio) {
    currentAudio.playbackRate = speed;
  }
}

function playAudioSequence() {
  console.log('playAudioSequence');

  document.getElementById('scrollable').children[currentIndex].style.color="black";
  document.getElementById('sections').children[currentIndex].style.color="black";

  var audio = fragments[currentIndex].audio;
  audio.currentTime = 0;
  audio.play();
  audio.playbackRate = currentSpeed;
  currentAudio = audio;
  console.log("Playing audio of duration " + audio.duration + " seconds");

  audio.onended = function() {
    audio.remove();
    currentIndex++;
    if (currentIndex < fragments.length) {
      playAudioSequence();
    }
  };
}

function formatDuration(duration) {
  console.log('formatDuration');

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
    session: sParameter,
    token: token,
  })
}).then(function(response) {
  return response.json();
}).then(function(data) {
  if (data.status != "ok") {
    return;
  }

  fragments = data['shas'].map(sha => {
    var foo = document.createElement('div');
    foo.style.color = "grey";
    document.getElementById('scrollable').appendChild(foo);

    var bar = document.createElement('div');
    bar.style.color = "grey";
    document.getElementById('sections').appendChild(bar);

    const audio = new Audio("data/output-" + sha + ".mp3");
    const textPromise = fetch("data/text-" + sha + ".txt").then(response => {
      if (response.ok) {
        return response.text();
      }
      throw new Error('Failed to fetch text for sha: ' + sha);
    }).then(e=> {
      foo.innerText=e;
      bar.innerText=e;
    });

    return {
      "sha": sha,
      "audio": audio,
      "textPromise": textPromise,
      "foo": foo, // TODO: Rename
      "bar": bar, // TODO: Rename
    };
  });

  const audioPromises = fragments.map(fragment => new Promise((resolve) => {
    fragment.audio.oncanplaythrough = resolve;
    fragment.audio.onerror = resolve;
  }));

  Promise.all([...audioPromises, ...fragments.map(fragment => fragment.text)])
    .then(() => {
      var duration = 0;
      for (let fragment of fragments) {
        duration += fragment.audio.duration;
      }
      document.getElementById('totalDuration').innerText = formatDuration(duration);
      console.log("Finished");
      playAudioSequence();
    })
    .catch(error => {
      console.error("An error occurred:", error);
    });

  sessionID = data['sessionID'];
  currentIndex = 0;
});

// TODO: Fix this perf
setInterval(() => {
  if (currentAudio) {
    var progress = currentAudio.currentTime / currentAudio.duration;

    if (currentIndex > 0) {
      fragments[currentIndex-1].foo.innerText = fragments[currentIndex-1].foo.innerText;
      //fragments[currentIndex-1].foo.style.fontWeight = "";
      fragments[currentIndex-1].foo.style.color = "grey";
    }

    var viewport = fragments[currentIndex].foo;
    var text = viewport.innerText;
    var textLength = text.length;
    var progressIdx = Math.floor(progress * textLength);
    var highlightedText = text.substring(0, progressIdx) + "<span style='background-color: yellow;'>" + text.substring(progressIdx, progressIdx+1) + "</span>" + text.substring(progressIdx+1);
    viewport.innerHTML = highlightedText;

    var previous = currentIndex > 0 ? fragments.slice(0, currentIndex).reduce((acc, e) => acc + e.audio.duration, 0) : 0;
    document.getElementById('durationPlayed').innerText = formatDuration(previous + currentAudio.currentTime);
  }
}, 1000/30);

document.addEventListener('keydown', function(event) {
  if (event.keyCode === 32) {
    event.preventDefault();

    playPauseAudio();
  }
});
