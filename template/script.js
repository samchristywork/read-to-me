<script>
  document.addEventListener('DOMContentLoaded', function() {
    const audioPlayer = document.querySelector('#audioPlayer');
    const postBody = document.querySelector('.post-body');
    const textBlocks = postBody.querySelectorAll('div');
    const timingSpans = postBody.querySelectorAll('span.hidden');
    const navigation = document.querySelectorAll('.navigation div');
    const autoscroll = document.querySelector('#autoscroll')

    let blockDurations = [];
    timingSpans.forEach(span => {
      blockDurations.push(parseInt(span.textContent, 10));
    });

    function updateTextStyles() {
      let currentTime = audioPlayer.currentTime * 1000;
      let accumulatedTime = 0;

      let first = true;
      textBlocks.forEach((block, index) => {
        accumulatedTime += blockDurations[index];

        if (currentTime >= accumulatedTime) {
          block.style.color = 'grey';
          navigation[index].style.color='grey';
        } else {
          block.style.color = 'black';
          if (first == true) {
            if (autoscroll.checked) {
              let y = block.offsetTop-70;
              window.scrollTo({top: y, behavior: 'smooth'});
            }
            navigation[index].style.color='black';
            first = false;
          } else {
            navigation[index].style.color='grey';
          }
        }
      });
    }

    audioPlayer.addEventListener('timeupdate', updateTextStyles);
  });
</script>`
