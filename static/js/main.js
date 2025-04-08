document
  .getElementById("convertForm")
  .addEventListener("submit", function (e) {
    e.preventDefault();
    const form = this;
    const progressDiv = document.getElementById("progress");
    const progressText = progressDiv.querySelector(".progress-text");
    const submitButton = form.querySelector("button");

    progressDiv.style.display = "block";
    progressText.textContent = "";
    submitButton.disabled = true;

    fetch(form.action, {
      method: "POST",
      body: new FormData(form),
    })
      .then((response) => response.json())
      .then((data) => {
        if (data && data.sessionId) {
          connectToEventSource(data.sessionId);
        } else if (data && data.error) {
          // Handle error from the server
          progressText.textContent = "Error: " + data.error;
          submitButton.disabled = false;
        }
      })
      .catch((error) => {
        progressText.textContent = "Error starting conversion";
        submitButton.disabled = false;
      });

    function connectToEventSource(sessionId) {
      const evtSource = new EventSource(`/progress?id=${sessionId}`);

      evtSource.onmessage = function (event) {
        const message = event.data;

        if (message === "DONE") {
          evtSource.close();
          window.location.reload();
          return;
        }

        progressText.textContent += message + "\n";

        if (
          message === "Conversion complete!" ||
          message.startsWith("Error:")
        ) {
          submitButton.disabled = false;
          evtSource.close();
        }
      };

      evtSource.onerror = function () {
        progressText.textContent +=
          "Connection lost. Check downloads page for your file.\n";
        submitButton.disabled = false;
        evtSource.close();
      };
    }
  });

const feedUrl =
  window.location.protocol + "//" + window.location.host + "/feed";
document.getElementById("feedUrl").textContent = feedUrl;

function copyFeedUrl() {
  navigator.clipboard
    .writeText(feedUrl)
    .then(() => alert("Feed URL copied to clipboard!"))
    .catch((err) => console.error("Error copying text: ", err));
}

if ("mediaSession" in navigator) {
  const audioElements = document.querySelectorAll("audio");
  audioElements.forEach((audio) => {
    audio.addEventListener("play", () => {
      navigator.mediaSession.metadata = new MediaMetadata({
        title: audio.dataset.title,
      });

      navigator.mediaSession.setActionHandler("previoustrack", null);
      navigator.mediaSession.setActionHandler("nexttrack", null);

      navigator.mediaSession.setActionHandler("seekbackward", () => {
        audio.currentTime = Math.max(audio.currentTime - 10, 0);
      });

      navigator.mediaSession.setActionHandler("seekforward", () => {
        audio.currentTime = Math.min(
          audio.currentTime + 30,
          audio.duration
        );
      });
    });
  });
}

function changePlaybackRate(select) {
  const audio = select.closest(".audio-player").querySelector("audio");
  audio.playbackRate = parseFloat(select.value);
}

function skipBackward(button) {
  const audio = button.closest(".audio-player").querySelector("audio");
  audio.currentTime = Math.max(audio.currentTime - 10, 0);
}

function skipForward(button) {
  const audio = button.closest(".audio-player").querySelector("audio");
  audio.currentTime = Math.min(audio.currentTime + 30, audio.duration);
}

// Keep audio playing when screen is locked - mobile only
document.addEventListener("visibilitychange", function () {
  const isMobile = "ontouchstart" in window && window.innerWidth <= 768;

  if (isMobile) {
    const audioElements = document.querySelectorAll("audio");
    audioElements.forEach((audio) => {
      if (audio.played.length > 0) {
        // Keep playing if already playing
        audio.play().catch(() => {});
      }
    });
  }
});

document.addEventListener(
  "play",
  function (e) {
    if (e.target.tagName.toLowerCase() === "audio") {
      const audioElements = document.querySelectorAll("audio");
      audioElements.forEach((audio) => {
        if (audio !== e.target && !audio.paused) {
          audio.pause();
        }
      });
    }
  },
  true
);
