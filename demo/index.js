window.onload = async () => {
  const iframe = document.querySelector("#simulator-iframe")
  const textarea = document.querySelector("#bulletml-textarea")
  const applyButton = document.querySelector("#apply-button")
  const recordButton = document.querySelector("#record-button")
  const downloadLink = document.querySelector("#download-link")
  const sampleSelector = document.querySelector("#sample-selector")
  const editorMessage = document.querySelector("#editor-message")

  const setEditorMessage = message => {
    editorMessage.textContent = message
    editorMessage.style.display = message ? "block" : "none"
  }

  const applySample = async name => {
    setEditorMessage("")

    const response = await fetch(`./${name}.xml`)
    const data = await response.text()
    textarea.value = data
    iframe.contentWindow.setBulletML(data)
  }

  const main = async () => {
    if (!iframe.contentWindow.setBulletML || !iframe.contentWindow.setErrorCallback) {
      setTimeout(main, 500)
      return
    }

    iframe.contentWindow.setErrorCallback(err => {
      setEditorMessage(err)
    })

    textarea.addEventListener("keydown", e => {
      if (e.shiftKey && e.keyCode === 13) {
        e.preventDefault()
        setEditorMessage("")
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
          const selectionEnd = e.currentTarget.value.charAt(e.currentTarget.selectionEnd - 1) === "\n" ? (e.currentTarget.selectionEnd - 1) : e.currentTarget.selectionEnd
          const suffixIdx = e.currentTarget.value.substring(selectionEnd).indexOf("\n")
          const target = e.currentTarget.value.substring(targetIdx, suffixIdx ===  -1 ? Infinity : (selectionEnd + suffixIdx))
          const suffix = e.currentTarget.value.substring(suffixIdx ===  -1 ? Infinity : (selectionEnd + suffixIdx))

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
      setEditorMessage("")
      iframe.contentWindow.setBulletML(textarea.value)
    })

    let recorder
    recordButton.addEventListener("click", () => {
      if (!recorder || recorder.state !== "recording") {
        const canvas = iframe.contentWindow.document.querySelector("canvas")
        const stream = canvas.captureStream()
        recorder = new MediaRecorder(stream, {mimeType: "video/webm;codecs=vp9"})

        recorder.addEventListener("dataavailable", async e => {
          try {
            const data = new Blob([e.data], {type: e.data.type})

            const buf = await data.arrayBuffer()
            const bin = new Uint8Array(buf)
            const ffmpeg = FFmpeg.createFFmpeg({
              corePath: new URL("thirdparty/ffmpeg.wasm-core.0.11.0/ffmpeg-core.js", document.location).href,
            })
            await ffmpeg.load()
            ffmpeg.FS("writeFile", "rec.webm", bin)
            await ffmpeg.run("-i", "rec.webm", "-vcodec", "copy", "rec.mp4")
            const converted = ffmpeg.FS("readFile", "rec.mp4")
            try {
              ffmpeg.exit()
            } catch (e) {
              console.log(e)
            }

            const result = new Blob([converted], { type: "video/mp4" })
            url = URL.createObjectURL(result)
            downloadLink.download = "bulletml-rec.mp4"
            downloadLink.href = url
            downloadLink.style.display = "inline"
          } finally {
            recordButton.removeAttribute("disabled")
            recordButton.textContent = "Record"
          }
        })

        recorder.addEventListener("error", e => {
          console.error(e)

          setEditorMessage("Failed to record simulator.")

          recordButton.removeAttribute("disabled")
          recordButton.textContent = "Record"
        })

        recorder.start()

        downloadLink.style.display = "none"
        recordButton.textContent = "Stop"
      } else {
        recorder.stop()

        recordButton.setAttribute("disabled", "true")
      }
    })

    sampleSelector.addEventListener("change", async e => {
      await applySample(e.currentTarget.value)
    })

    const query = Object.fromEntries(location.search.substring(1,).split('&').map((kv) => kv.split('=')))

    if (typeof query.sample === "string" && query.sample.match(/^[a-z0-9\-]+$/)) {
      sampleSelector.value = query.sample
    }

    await applySample(sampleSelector.value)
  }

  await main()
}
