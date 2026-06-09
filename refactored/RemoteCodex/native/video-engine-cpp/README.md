# Velox Video Engine C++ Scaffold

Questo modulo prepara il motore pesante in C++ per la composizione video.

Obiettivi immediati:
- ricevere un payload con `video_name`, `script_text`, `voiceover_paths` e `scenes_json`
- normalizzare il progetto scena per scena
- produrre un artefatto video finale quando il compositore FFmpeg/GPU sarà pronto

Estensione base già supportata:
- `video_mode=clip_stock` per concat semplice di clip iniziali, clip di contenuto e stock footage
- `intro_clip_paths`, `stock_clip_paths` e `clip_segments`
- mux audio finale con `voiceover_paths`

Stato attuale:
- contract definito
- esempio payload disponibile
- entrypoint minimale presente
- rendering reale ancora da implementare
