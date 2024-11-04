function resetPassword() {
  fetch('/reset-password', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({
      email: document.querySelector("#email").value,
    })
  }).then(function(response) {
    return response.json();
  }).then(function(data) {
    if(data.status=="ok") {
      email: document.querySelector(".container").innerHTML="<h1>Email Sent</h1>"
    } else {
      document.querySelector(".error").innerText="Password reset failed.";
    }
  });
}
