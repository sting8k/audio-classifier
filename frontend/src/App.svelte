<script lang="ts">
  // SCAFFOLD placeholder: the ui peer replaces this with the 4 screens.
  import { DefaultSettings } from '../wailsjs/go/main/App.js'
  let exts = $state('')
  DefaultSettings().then((s) => (exts = s.extensions.join(', ')))
</script>

<main><h1>LangID Batch</h1><p>{exts}</p></main>
