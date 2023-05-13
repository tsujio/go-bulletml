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

    textarea.addEventListener("keydown", e => {
      if (e.shiftKey && e.keyCode === 13) {
        e.preventDefault()
        iframe.contentWindow.setBulletML(e.currentTarget.value)
      }

      if (e.keyCode === 9) {
        e.preventDefault()

        const tab = "    "
        let result = ""

        if (e.currentTarget.selectionStart === e.currentTarget.selectionEnd && !e.shiftKey) {
          const prefix = e.currentTarget.value.substring(0, e.currentTarget.selectionStart)
          const suffix = e.currentTarget.value.substring(e.currentTarget.selectionStart)
          result = prefix + tab + suffix
        } else {
          const targetIdx = e.currentTarget.value.substring(0, e.currentTarget.selectionStart).lastIndexOf("\n") + 1
          const prefix = e.currentTarget.value.substring(0, targetIdx)
          const suffixIdx = e.currentTarget.value.substring(e.currentTarget.selectionEnd).indexOf("\n")
          const target = e.currentTarget.value.substring(targetIdx, suffixIdx ===  -1 ? Infinity : (e.currentTarget.selectionEnd + suffixIdx))
          const suffix = e.currentTarget.value.substring(suffixIdx ===  -1 ? Infinity : (e.currentTarget.selectionEnd + suffixIdx))

          result = prefix

          if (e.shiftKey) {
            result += target.replaceAll(/^ {1,4}/gm, "")
          } else {
            result += target.replaceAll(/^/gm, tab)
          }

          result += suffix
        }

        const end = e.currentTarget.selectionEnd
        const origLen = e.currentTarget.value.length
        const origLineN = (e.currentTarget.value.substring(0, e.currentTarget.selectionEnd).match(/\n/g) || []).length
        e.currentTarget.value = result
        e.currentTarget.selectionEnd = end + result.length - origLen
        const newLineN = (e.currentTarget.value.substring(0, e.currentTarget.selectionEnd).match(/\n/g) || []).length
        if (origLineN !== newLineN) {
          const idx = e.currentTarget.value.substring(e.currentTarget.selectionEnd).indexOf("\n")
          if (idx !== -1) {
            e.currentTarget.selectionStart = e.currentTarget.selectionEnd += idx + 1
          }
        }
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
