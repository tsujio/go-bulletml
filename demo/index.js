window.onload = async () => {
  const title = document.querySelector("#title")
  const iframe = document.querySelector("#simulator-iframe")
  const textarea = document.querySelector("#bulletml-textarea")
  const applyButton = document.querySelector("#apply-button")
  const recordButton = document.querySelector("#record-button")
  const shareButton = document.querySelector("#share-button")
  const downloadLink = document.querySelector("#download-link")
  const sampleSelector = document.querySelector("#sample-selector")
  const editorMessage = document.querySelector("#editor-message")

  title.addEventListener("click", () => {
    document.location.href = new URL("/", document.location).href
  })

  const setEditorMessage = message => {
    editorMessage.textContent = message
    editorMessage.style.display = message ? "block" : "none"
  }

  const applySample = async name => {
    setEditorMessage("")

    const response = await fetch(`./${name}.xml`)
    if (!response.ok) {
      const text = await response.text()
      console.error(text)
      setEditorMessage(`Failed to fetch sample: ${text}`)
      return
    }
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
      if (e.key === "Enter") {
        if (e.shiftKey) {
          e.preventDefault()
          setEditorMessage("")
          iframe.contentWindow.setBulletML(e.currentTarget.value)
        } else {
          const value = e.currentTarget.value
          const prefix = value.substring(0, e.currentTarget.selectionStart)
          const suffix = value.substring(e.currentTarget.selectionEnd)
          if (value.charAt(e.currentTarget.selectionStart) === "\n") {
            e.preventDefault()
            const complement = "\n" + prefix.substring(prefix.lastIndexOf("\n") + 1).match(/^( *)/)[1]
            const selectionStart = e.currentTarget.selectionStart
            e.currentTarget.value = prefix + complement + suffix
            e.currentTarget.selectionStart = e.currentTarget.selectionEnd = selectionStart + complement.length
          }
        }

        return
      }

      if (e.key === "Tab") {
        e.preventDefault()

        const value = e.currentTarget.value
        const selectionStart = e.currentTarget.selectionStart
        const selectionEnd = e.currentTarget.selectionEnd
        const tab = "    "
        let result = ""

        if (selectionStart === selectionEnd && !e.shiftKey) {
          const prefix = value.substring(0, selectionStart)
          const suffix = value.substring(selectionStart)
          result = prefix + tab + suffix
        } else {
          const targetIdx = value.substring(0, selectionStart).lastIndexOf("\n") + 1
          const prefix = value.substring(0, targetIdx)
          const suffixIdx = value.indexOf("\n", selectionStart === selectionEnd ? selectionEnd : selectionEnd - 1)
          const target = value.substring(targetIdx, suffixIdx ===  -1 ? Infinity : suffixIdx)
          const suffix = value.substring(suffixIdx ===  -1 ? Infinity : suffixIdx)

          result = prefix

          if (e.shiftKey) {
            result += target.replaceAll(/^ {1,4}/gm, "")
          } else {
            result += target.replaceAll(/^/gm, tab)
          }

          result += suffix
        }

        e.currentTarget.value = result

        let offset = 0
        if (e.currentTarget.value.length < value.length) {
          const col = selectionEnd - value.lastIndexOf("\n", selectionEnd - 1) - 1
          if (col < tab.length) {
            offset = tab.length - col
          }
        }

        e.currentTarget.selectionStart = e.currentTarget.selectionEnd = selectionEnd + result.length - value.length + offset

        return
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

        recorder = null
        for (const mimeType of ["video/webm;codecs=vp9", "video/webm;codecs=vp8"]) {
          if (MediaRecorder.isTypeSupported(mimeType)) {
            console.log(`MIME type ${mimeType} supported on this browser.`)
            recorder = new MediaRecorder(stream, {mimeType: mimeType})
            break
          }
        }

        if (!recorder) {
          console.error("Supported MIME type not found")
          setEditorMessage("Recording not supported on this browser.")
          return
        }

        const chunks = []
        recorder.addEventListener("dataavailable", async e => {
          chunks.push(e.data)
        })

        recorder.addEventListener("stop", async () => {
          try {
            const data = new Blob(chunks, {type: recorder.mimeType})

            if (data.size === 0) {
              setEditorMessage("Failed to record video (data have no length)")
              return
            }

            const buf = await data.arrayBuffer()
            const bin = new Uint8Array(buf)
            const ffmpeg = FFmpeg.createFFmpeg({
              corePath: new URL("thirdparty/ffmpeg.wasm-core.0.11.0/ffmpeg-core.js", document.location).href,
            })
            await ffmpeg.load()
            ffmpeg.FS("writeFile", "rec.webm", bin)
            await ffmpeg.run("-i", "rec.webm", "-vcodec", "copy", "-an", "rec.mp4")
            const converted = ffmpeg.FS("readFile", "rec.mp4")
            try {
              ffmpeg.FS("unlink", "rec.webm")
              ffmpeg.FS("unlink", "rec.mp4")
              ffmpeg.exit()
            } catch (e) {
              console.warn(e)
            }

            let result, filename
            if (converted.length === 0) {
              result = data
              filename = "bulletml-rec.webm"
              setEditorMessage("Failed to convert video to mp4, but you can download webm video.")
            } else {
              result = new Blob([converted], {type: "video/mp4"})
              filename = "bulletml-rec.mp4"
              setEditorMessage("")
            }

            url = URL.createObjectURL(result)
            downloadLink.download = filename
            downloadLink.href = url
            downloadLink.style.display = "inline"
          } catch (e) {
            console.error(e)
            setEditorMessage(`Failed to convert video (maybe cannot work well on mobile devices): ${e}`)
          } finally {
            recorder = null
            recordButton.removeAttribute("disabled")
            recordButton.textContent = "Record"
          }
        })

        recorder.addEventListener("error", e => {
          console.error(e)

          setEditorMessage(`Error occurred in recorder: ${e}`)

          recordButton.removeAttribute("disabled")
          recordButton.textContent = "Record"
        })

        recorder.start()

        downloadLink.style.display = "none"
        recordButton.textContent = "Stop"
      } else {
        recorder.stop()

        recordButton.setAttribute("disabled", "true")
        recordButton.textContent = "Converting"
      }
    })

    shareButton.addEventListener("click", async () => {
      const id = btoa(Math.random()).substring(3).replaceAll(/\+|\/|=/g, "")

      let userId = localStorage.getItem("USER_ID")
      if (!userId) {
        userId = crypto.randomUUID()
        localStorage.setItem("USER_ID", userId)
      }

      const data = textarea.value + `\n<!-- ${JSON.stringify({userId: userId})} -->`

      const response = await fetch(`https://storage.googleapis.com/bulletml-simulator-share/${id}.xml`, {
        method: "PUT",
        headers: {
          "Content-Type": "application/xml",
        },
        body: data,
      })

      if (!response.ok) {
        const text = await response.text()
        console.error(text)
        setEditorMessage(`Failed to save data: ${text}`)
        return
      }

      const u = new URL("/", document.location)
      u.searchParams.set("id", id)
      const url = u.href

      const selectorId = sampleSelector.getAttribute("id")
      const parent = sampleSelector.parentElement
      sampleSelector.remove()
      const elem = document.createElement("input")
      elem.setAttribute("type", "text")
      elem.setAttribute("readonly", "true")
      elem.setAttribute("value", url)
      elem.setAttribute("id", selectorId)
      parent.appendChild(elem)
      elem.focus()
      elem.select()

      try {
        await navigator.clipboard.writeText(url)
      } catch (e) {
        console.warn(e)
      }

      window.history.pushState({}, "", url)

      setEditorMessage("")
    })

    sampleSelector.addEventListener("change", async e => {
      await applySample(e.currentTarget.value)
    })

    const query = Object.fromEntries(location.search.substring(1,).split('&').map((kv) => kv.split('=')))

    if (typeof query.sample === "string" && query.sample.match(/^[a-z0-9\-]+$/)) {
      sampleSelector.value = query.sample
      await applySample(sampleSelector.value)
    } else if (typeof query.id === "string" && query.id.match(/^[a-zA-Z0-9]+$/)) {
      const response = await fetch(`https://storage.googleapis.com/bulletml-simulator-share/${query.id}.xml`)
      if (!response.ok) {
        console.error(await response.text())
        setEditorMessage(`Failed to fetch data (id=${query.id})`)
        return
      }

      let data = await response.text()

      const idx = data.lastIndexOf("\n")
      if (idx !== -1) {
        const m = data.substring(idx).match(/^\n<!-- ([^ ]+) -->$/)
        if (m) {
          try {
            JSON.parse(m[1])
            data = data.substring(0, idx)
          } catch (e) {
          }
        }
      }

      textarea.value = data
      iframe.contentWindow.setBulletML(textarea.value)
    } else {
      await applySample(sampleSelector.value)
    }
  }

  const resize = () => {
    if (window.innerWidth < 1200) {
      iframe.style.height = Math.min(iframe.clientWidth * 4 / 3, 640) + "px"
    } else {
      iframe.style.height = "100%"
    }
  }

  window.addEventListener("resize", resize)

  resize()

  await main()
}
