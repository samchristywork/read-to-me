function changePassword() {
  const urlParams = new URLSearchParams(window.location.search);
  fetch('/change-password', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({
      username: urlParams.get('u'),
      key: urlParams.get('k'),
      password: document.querySelector("#password").value
    })
  }).then(function(response) {
    return response.json();
  }).then(function(data) {
    if(data.status=="ok") {
      window.location.href="/";
    } else {
      document.querySelector(".error").innerText="Password Change Failed";
    }
  });
}
