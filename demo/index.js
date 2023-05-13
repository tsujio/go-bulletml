window.onload = () => {
  const iframe = document.querySelector("#simulator-iframe")
  const textarea = document.querySelector("#bulletml-textarea")
  const applyButton = document.querySelector("#apply-button")
  const sampleSelector = document.querySelector("#sample-selector")

  const applySample = name => {
    fetch(`./${name}.xml`)
      .then(r => r.text())
      .then(d => {
        textarea.value = d
        iframe.contentWindow.setBulletML(d)
      })
  }

  const main = () => {
    if (!iframe.contentWindow.setBulletML) {
      setTimeout(main, 500)
      return
    }

    textarea.addEventListener("keypress", e => {
      if (e.shiftKey && e.keyCode === 13) {
        e.preventDefault()
        iframe.contentWindow.setBulletML(e.currentTarget.value)
      }
    })

    applyButton.addEventListener("click", () => {
      iframe.contentWindow.setBulletML(textarea.value)
    })

    sampleSelector.addEventListener("change", e => {
      applySample(e.currentTarget.value)
    })

    const query = Object.fromEntries(location.search.substring(1,).split('&').map((kv) => kv.split('=')))

    if (typeof query.sample === "string" && query.sample.match(/^[a-z0-9\-]+$/)) {
      sampleSelector.value = query.sample
    }

    applySample(sampleSelector.value)
  }

  main()
}
