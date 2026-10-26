# Voice control

Chefs' hands are full or dirty, so the pass, the stations and HACCP checks can be run by voice. Hold the **Hold to talk** button (or the space bar), say the command, and let go. Kishtin says what it did, and shows it on screen.

Everything voice can do can also be done by touch, and every command can be typed instead of spoken.

## Where speech goes

Nowhere outside the building.

- The browser records while the button is held and sends the audio to the Kishtin server on the kitchen network.
- The server passes it to a **speech-to-text sidecar** (a [whisper.cpp](https://github.com/ggml-org/whisper.cpp) server) running on the same machine or the kitchen network. Kishtin refuses to use a sidecar address outside the local network.
- Replies are spoken by a **text-to-speech sidecar** (a [Piper](https://github.com/OHF-Voice/piper1-gpl) HTTP server), on the same terms.
- The audio is not stored, unless a manager sets **Keep recordings of voice commands** in Settings; then it is kept on the server for that many days and deleted after. See [recording.md](recording.md#voice). The transcript is always kept, in the voice log, with what it was taken to mean, what happened and the device.

Without a text-to-speech sidecar, a device speaks replies with a voice built into the device, but only one that runs on the device itself (some browser voices send text to the cloud, and those are never used). Each device can turn spoken replies off on the Voice page.

## What you can say

| Say | Does |
|---|---|
| "Fire table 4 mains" | Fires the course and sets its target time. Leave out the course ("fire table 4") to fire the next held course. |
| "Hold table 4 mains" | Holds a fired course again, while nothing in it has started. |
| "What's next on grill?" | Reads the scheduler's next starts for that station. |
| "Burgers on, tables 4 and 7" | Marks those items cooking. "Two burgers on table 4" starts two. With no table, it starts what the scheduler says to start now. |
| "Table 7 ready" | Marks the fired course ready at the pass, once every item in it has been started. |
| "86 salmon", "salmon back on" | Marks a dish sold out, or back on. |
| "Walk-in fridge 3.4 degrees" | Logs a fridge or freezer reading and says pass, borderline or fail. "Chest freezer minus 18" works too. |
| "Core temp chicken 78" | Logs a cooking check. "Reheat soup 74" and "hot hold curry 65" log reheating and hot holding. A dish on the menu gets its own limits. |
| "Void burger table 4" | Voids items. Kishtin asks how many (if there is more than one), which ones (if some are cooking and some are waiting), and why, then needs **"confirm void"**. Head chef or manager only. "Void 2 burgers table 4 because dropped" says it all at once. |
| "Delivery chilled chicken from Synthetic Farm 3 degrees" | Logs a delivery. Whatever is not said is asked, one question at a time: who it is from, what it is, chilled, frozen or ambient, the temperature, whether the packaging is intact, whether it is all in date, and whether the delivery vehicle was clean and cold. "Packaging fine, in date, van clean" can be said up front ("vehicle dirty" fails the delivery). |
| "Cooling rice cooked at 14:30" | Logs food going into the fridge to cool, now. "Cooked 90 minutes ago" and "cooked at 2 pm" work too. A time later than now is taken as yesterday. |
| "Batch grape syrup 5 days" | Logs a batch label made today, with its use-by date. "Label 2 litres of stock 3 days" records the quantity. |
| "Floors done" | Ticks a cleaning task from the list. "Cleaning" on its own asks which task. |
| "Confirm", "cancel" | Answers a question. |

![Voice](screenshots/voice.png)

An order with a name can be said by its name: "fire Aoife mains", "Aoife ready", "burgers on for Aoife". If two open orders share a name, Kishtin asks which.

Numbers can be said as words or digits ("three point four", "minus eighteen"). Common mishearings are expected: "fryer table 4" is read as "fire table 4", "table for" as table 4, and "what's next on fire" as the fryer.

## Questions and confirmations

Kishtin asks rather than guesses.

- If a table is not open but sounds like one that is, it asks: "Table 4 or Table 14?" Answer with the table ("fourteen"), "the second one", or tap the answer on screen.
- If a dish could be more than one ("chicken" with both chicken supreme and chicken wings), it asks which.
- A HACCP reading that fails (or is borderline, when the rules say borderline readings need an action) is not saved straight away. Kishtin says the result and asks what you did about it. Say the action ("moved the stock to the cold room"), then **"confirm"** to save it, or "cancel" to save nothing. The buttons on screen do the same.
- A void is read back with its reason ("Void 1 Burger (cooking) for Table 4: dropped. Say confirm void, or cancel.") and nothing is voided until you say **"confirm void"**.
- Saving a failed reading and voiding need the word **"confirm"**. "Yes" or "go ahead" are not enough there, because they are easily said to someone else in a busy kitchen.
- A record logged in steps asks one question at a time. "Cancel" at any step saves nothing.
- A question waits for one minute, then lapses. Each person has their own: one chef's answer never answers another's question.

The same permissions apply as on the screens. A front-of-house login cannot fire a course by voice, for example, and Kishtin says so.

## Setting it up

1. Run a whisper.cpp server on the Kishtin machine. Start it with `--convert` so it accepts the browser's WebM and Ogg audio (it needs `ffmpeg`), and pick a small English model that runs well on the machine, for example:

   ```sh
   whisper-server -m models/ggml-base.en.bin --host 127.0.0.1 --port 8178 --convert
   ```

2. Optionally run a Piper HTTP server for spoken replies, for example on port 5000.
3. Point Kishtin at them in `kishtin.toml` (or with `KISHTIN_STT_URL` and `KISHTIN_TTS_URL`) and restart `kishtind`:

   ```toml
   [speech]
   stt_url = "http://127.0.0.1:8178"
   tts_url = "http://127.0.0.1:5000"
   ```

4. Browsers only allow the microphone on a secure (HTTPS) connection, or on the Kishtin computer itself. Tablets connected by QR code use HTTPS with Kishtin's own certificate, so voice works on them; see [Connecting a tablet or phone](screens.md#connecting-a-tablet-or-phone). A device that opens Kishtin over plain HTTP shows "The microphone needs a secure connection", and typed commands still work.

The Voice page shows whether spoken commands and spoken replies are set up, and whether this device's microphone can be used.

Kishtin sends whisper a short prompt with what can be said right now (open tables, stations, fridges and dishes), which helps it pick menu words over similar-sounding ones.

### Microphones and pedals

Kitchen noise is the main accuracy problem. Use close-talk headsets or a directional microphone at the pass, and test with the real kitchen running.

On a kitchen screen, holding the **space bar** works like holding the button, so a USB foot pedal or a headset button that can be set to send the space key works hands-free. A short tap on the button starts listening and a second tap sends, for when holding is awkward. Recordings stop after 20 seconds.

## Hands-free: the wake word

For when even a button is too much, a kitchen screen can listen for the wake word, **"Kishtin"**: "Kishtin, fire table 4 mains", "Kishtin, walk-in fridge 3 degrees".

- It is **off** until someone turns it on (**Hands-free: listen for "Kishtin"** under Hold to talk), on that device only. Reloading the page, switching to another app or locking the tablet turns it off again.
- While it is on, the screen shows **Listening for "Kishtin"**, then **Hearing…** while someone speaks and **Working…** while the server checks.
- The browser picks out phrases by loudness alone. It learns the room's background noise (fans, fryers) and waits for sound clearly above it. Each phrase goes to the Kishtin server's own speech-to-text, never anywhere else.
- If a phrase does not start with the wake word, it is thrown away: nothing is done, nothing is logged, and what was said is not sent back to the screen.
- "Kishtin" on its own replies "Yes?"; the next phrase within 8 seconds is the command.
- Answers need the wake word too: "Kishtin, fourteen", "Kishtin, confirm void". A "confirm" said across the kitchen does nothing.
- It does not listen while push-to-talk is in use or while a reply is being spoken, so it does not hear itself.

Things to know before relying on it:

- Every phrase anyone says near the tablet is turned into text on the server, so the server works harder than with push-to-talk. Watch it during a busy service.
- In a loud kitchen it can miss the wake word, or hear it where it was not said. Test it during a real service, with the microphone where it will be used, before relying on it. Push-to-talk stays the dependable way.
- Each phrase is cut off after 8 seconds, and 8 seconds of unbroken sound is treated as background noise.
- The wake word can be changed with `speech.wake_words` in kishtin.toml. Choose a word of 4 letters or more that is not said in normal kitchen talk ("chef" or "service" would trigger all night).

## Commands

```sh
kishtin say fire table 4 mains          # a typed command, same grammar
kishtin say --audio command.wav         # transcribed by the server's sidecar
kishtin say --audio phrase.wav --hands-free   # as hands-free: ignored without the wake word
kishtin say confirm                     # answers a waiting question
kishtin voice status                    # what is set up, and any waiting question
kishtin voice log                       # recent commands: heard, outcome, reply
```

API: `GET /api/voice`, `POST /api/voice/command`, `POST /api/voice/audio` (with `?hands_free=true` for hands-free phrases), `POST /api/voice/speak` and `GET /api/voice/log`.

## What voice does not do

- Hands-free listening is not a proper wake-word model: it sends every phrase to speech-to-text and checks the words. It has not been tested in a real kitchen.
- No replacing an item (Change on the screen), and nothing that deletes.
- No docket photo for a delivery: add it on the screen.
- It understands a fixed set of commands, not free conversation. "Get table 4's mains going" is not understood; "fire table 4 mains" is.
- It has not yet been tuned on recordings from a real kitchen.
