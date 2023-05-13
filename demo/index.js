window.onload = () => {
  const iframe = document.querySelector("#simulator-iframe")
  const textarea = document.querySelector("#bulletml-textarea")
  const applyButton = document.querySelector("#apply-button")

  applyButton.addEventListener("click", () => {
    console.log(textarea.value)

    iframe.contentWindow.setBulletML(textarea.value)
  })
}
