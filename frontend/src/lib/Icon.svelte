<!-- Icône heroicons (outline 24) en SVG inline, hérite de la couleur et de la taille du texte. -->
<script>
  import i_film from 'heroicons/24/outline/film.svg?raw';
  import i_tv from 'heroicons/24/outline/tv.svg?raw';
  import i_magnifying_glass from 'heroicons/24/outline/magnifying-glass.svg?raw';
  import i_queue_list from 'heroicons/24/outline/queue-list.svg?raw';
  import i_language from 'heroicons/24/outline/language.svg?raw';
  import i_exclamation_triangle from 'heroicons/24/outline/exclamation-triangle.svg?raw';
  import i_folder from 'heroicons/24/outline/folder.svg?raw';
  import i_folder_open from 'heroicons/24/outline/folder-open.svg?raw';
  import i_speaker_wave from 'heroicons/24/outline/speaker-wave.svg?raw';
  import i_check_circle from 'heroicons/24/outline/check-circle.svg?raw';
  import i_viewfinder_circle from 'heroicons/24/outline/viewfinder-circle.svg?raw';
  import i_bolt from 'heroicons/24/outline/bolt.svg?raw';
  import i_arrow_path from 'heroicons/24/outline/arrow-path.svg?raw';
  import i_x_circle from 'heroicons/24/outline/x-circle.svg?raw';
  import i_clock from 'heroicons/24/outline/clock.svg?raw';
  import i_adjustments_horizontal from 'heroicons/24/outline/adjustments-horizontal.svg?raw';
  import i_star from 'heroicons/24/outline/star.svg?raw';
  import i_stop from 'heroicons/24/outline/stop.svg?raw';
  import i_globe_alt from 'heroicons/24/outline/globe-alt.svg?raw';
  import i_pencil from 'heroicons/24/outline/pencil.svg?raw';
  import i_arrow_up_tray from 'heroicons/24/outline/arrow-up-tray.svg?raw';
  import i_rocket_launch from 'heroicons/24/outline/rocket-launch.svg?raw';
  import i_musical_note from 'heroicons/24/outline/musical-note.svg?raw';
  import i_light_bulb from 'heroicons/24/outline/light-bulb.svg?raw';
  import i_chat_bubble_left_ellipsis from 'heroicons/24/outline/chat-bubble-left-ellipsis.svg?raw';
  import i_arrow_down_tray from 'heroicons/24/outline/arrow-down-tray.svg?raw';
  import i_document_text from 'heroicons/24/outline/document-text.svg?raw';
  import i_command_line from 'heroicons/24/outline/command-line.svg?raw';
  import i_cpu_chip from 'heroicons/24/outline/cpu-chip.svg?raw';
  import i_trash from 'heroicons/24/outline/trash.svg?raw';
  import i_play from 'heroicons/24/outline/play.svg?raw';
  import i_check from 'heroicons/24/outline/check.svg?raw';
  import i_arrow_uturn_left from 'heroicons/24/outline/arrow-uturn-left.svg?raw';
  import i_cog_6_tooth from 'heroicons/24/outline/cog-6-tooth.svg?raw';
  import i_arrow_top_right_on_square from 'heroicons/24/outline/arrow-top-right-on-square.svg?raw';

  const ICONS = {
    'film': i_film,
    'tv': i_tv,
    'magnifying-glass': i_magnifying_glass,
    'queue-list': i_queue_list,
    'language': i_language,
    'exclamation-triangle': i_exclamation_triangle,
    'folder': i_folder,
    'folder-open': i_folder_open,
    'speaker-wave': i_speaker_wave,
    'check-circle': i_check_circle,
    'viewfinder-circle': i_viewfinder_circle,
    'bolt': i_bolt,
    'arrow-path': i_arrow_path,
    'x-circle': i_x_circle,
    'clock': i_clock,
    'adjustments-horizontal': i_adjustments_horizontal,
    'star': i_star,
    'stop': i_stop,
    'globe-alt': i_globe_alt,
    'pencil': i_pencil,
    'arrow-up-tray': i_arrow_up_tray,
    'rocket-launch': i_rocket_launch,
    'musical-note': i_musical_note,
    'light-bulb': i_light_bulb,
    'chat-bubble-left-ellipsis': i_chat_bubble_left_ellipsis,
    'arrow-down-tray': i_arrow_down_tray,
    'document-text': i_document_text,
    'command-line': i_command_line,
    'cpu-chip': i_cpu_chip,
    'trash': i_trash,
    'play': i_play,
    'check': i_check,
    'arrow-uturn-left': i_arrow_uturn_left,
    'cog-6-tooth': i_cog_6_tooth,
    'arrow-top-right-on-square': i_arrow_top_right_on_square,
    'rocket': i_rocket_launch,
    'cog': i_cog_6_tooth,
    'download': i_arrow_down_tray,
  };

  let { name, class: cls = '' } = $props();
</script>

<span class="icon {cls}" aria-hidden="true">{@html ICONS[name] || ''}</span>
