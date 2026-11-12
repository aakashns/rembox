// The install command's Copy button, shown once there's a clipboard to copy to.
const button = document.querySelector("#copy");
const command = document.querySelector("#install-command");
const status = document.querySelector("#copy-status");

if (button && command && status && navigator.clipboard) {
  button.hidden = false;
  let reset;
  button.addEventListener("click", async () => {
    clearTimeout(reset);
    try {
      await navigator.clipboard.writeText(command.textContent);
      button.textContent = "Copied!";
      status.textContent = "Install command copied to clipboard.";
    } catch {
      const selection = window.getSelection();
      const range = document.createRange();
      range.selectNodeContents(command);
      selection?.removeAllRanges();
      selection?.addRange(range);
      status.textContent = "Select and copy the highlighted command.";
      button.textContent = "Select & copy";
    }
    reset = setTimeout(() => {
      button.textContent = "Copy";
    }, 3000);
  });
}
