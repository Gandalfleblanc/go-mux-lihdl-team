# GO Mux LiHDL Team

Application desktop Wails v2 (Go + Svelte) pour muxer des fichiers `.mkv` selon les normes LiHDL, avec **MKVToolNix embarqué** (pas besoin d'installation externe).

## Stack

- Go 1.23+
- [Wails v2.12.0](https://wails.io)
- Svelte 3
- MKVToolNix (`mkvmerge`) binaires embarqués par plateforme

## Commandes

```bash
# Dev avec hot reload
~/go/bin/wails dev

# Build prod (plateforme courante)
~/go/bin/wails build -platform darwin/arm64
```

## Normes LiHDL

Les règles de renommage (pistes audio, sous-titres, piste vidéo, nom de fichier) sont figées et appliquées automatiquement. Voir `docs/` pour le détail.

## Licence

Usage interne — équipe LiHDL.
