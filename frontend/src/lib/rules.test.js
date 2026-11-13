import { describe, it, expect } from 'vitest';
import {
  detectEpisode, detectSourceType, parsePsaSourceInfo, parseSupplyInfo, looksLikeAD,
  inferAudioLabel, inferSubLabel, mapLangCode, dotify, subLabelPriority,
  cleanQueryFromFilename, langFlag,
} from './rules.js';

describe('detectEpisode', () => {
  it('détecte SxxExx avec points', () => {
    expect(detectEpisode('The.Terminal.List.S02E01.1080p.WEBRip.x265-PSA.mkv')).toBe('S02E01');
  });
  it('accepte les underscores (Dallas_S03E01_)', () => {
    expect(detectEpisode('Dallas_S03E01_.mkv')).toBe('S03E01');
  });
  it('complète les numéros à 2 chiffres', () => {
    expect(detectEpisode('Show.S1E5.mkv')).toBe('S01E05');
  });
  it('ne détecte rien sur un film', () => {
    expect(detectEpisode('Fortress.2012.1080p.BluRay.mkv')).toBe('');
  });
  it('ne prend pas un SxxExx collé à un mot', () => {
    expect(detectEpisode('ClassS01E01.mkv')).toBe('');
  });
});

describe('cleanQueryFromFilename', () => {
  it('garde le titre avant SxxExx', () => {
    expect(cleanQueryFromFilename('The.Terminal.List.S02E01.1080p.10bit.WEBRip.6CH.x265.HEVC-PSA.mkv')).toBe('The.Terminal.List');
  });
  it("retire l'année avant SxxExx", () => {
    expect(cleanQueryFromFilename('Boston.Blue.2025.S01E15.1080p.mkv')).toBe('Boston.Blue');
  });
  it("coupe un film à l'année", () => {
    expect(cleanQueryFromFilename('In.the.Name.of.Ben.Hur.2016.1080p.BluRay.mkv')).toBe('In.the.Name.of.Ben.Hur');
  });
  it('gère les underscores', () => {
    expect(cleanQueryFromFilename('Dallas_S03E01_.mkv')).toBe('Dallas');
  });
});

describe('detectSourceType', () => {
  it('REMUX CUSTOM prioritaire', () => {
    expect(detectSourceType('Homeland.S01E01.CUSTOM.MULTi.1080p.BLURAY.REMUX.mkv')).toBe('REMUX CUSTOM');
  });
  it('WEB-DL sous forme dotée', () => {
    expect(detectSourceType('Film.2024.1080p.AMZN.WEB.DL.DDP5.1.mkv')).toBe('WEB-DL');
  });
  it('WEBRip', () => {
    expect(detectSourceType('Show.S01E01.1080p.WEBRip.x265.mkv')).toBe('WEBRip');
  });
  it('COMPLETE BluRay exige les deux mots', () => {
    expect(detectSourceType('Film.2020.COMPLETE.BLURAY.mkv')).toBe('COMPLETE BluRay');
    expect(detectSourceType('Film.2020.BLURAY.mkv')).toBe('');
  });
});

describe('parsePsaSourceInfo / parseSupplyInfo', () => {
  it('reconnaît une PSA et son codec', () => {
    expect(parsePsaSourceInfo('Sheriff.Country.S02E02.1080p.10bit.WEBRip.6CH.x265.HEVC-PSA.mkv'))
      .toEqual({ isPSA: true, source: 'WEBRip', videoCodec: 'x265', team: 'PSA' });
  });
  it("lit la team et le flag d'un SUPPLY", () => {
    expect(parseSupplyInfo('Show.S01E01.MULTi.VFF.1080p.WEB.H264-SUPPLY.mkv'))
      .toEqual({ team: 'SUPPLY', langFlag: 'MULTi.VFF' });
  });
  it('VOSTFR seul → FASTSUB.VOSTFR', () => {
    expect(parseSupplyInfo('Show.S01E01.VOSTFR.1080p.WEB-FW.mkv').langFlag).toBe('FASTSUB.VOSTFR');
  });
});

describe('inferAudioLabel', () => {
  const fr = (extra) => ({ language: 'fre', codec_id: 'A_AC3', audio_channels: 6, ...extra });
  it('FR par défaut = VFF', () => {
    expect(inferAudioLabel(fr())).toBe('FR VFF : AC3 5.1');
  });
  it('« descriptive » dans le nom → FR AD', () => {
    expect(inferAudioLabel(fr({ track_name: 'French Descriptive' }))).toBe('FR AD : AC3 5.1');
  });
  it('« Canadian » → FR VFQ', () => {
    expect(inferAudioLabel(fr({ track_name: 'French Canadian' }))).toBe('FR VFQ : AC3 5.1');
  });
  it("le codec vient du codec_id, pas du nom de piste", () => {
    expect(inferAudioLabel(fr({ track_name: 'VFF E-AC3 5.1' }))).toBe('FR VFF : AC3 5.1');
  });
  it('EAC3 5.1 JOC → ATMOS', () => {
    expect(inferAudioLabel({ language: 'eng', codec_id: 'A_EAC3', audio_channels: 6, mi_format_features: 'JOC' }))
      .toBe('ENG VO : EAC3 5.1 ATMOS');
  });
  it('stéréo EAC3 anglaise', () => {
    expect(inferAudioLabel({ language: 'eng', codec_id: 'A_EAC3', audio_channels: 2 })).toBe('ENG VO : EAC3 2.0');
  });
});

describe('inferSubLabel', () => {
  it('FR texte par défaut = Full SRT', () => {
    expect(inferSubLabel({ language: 'fre', codec_id: 'S_TEXT/UTF8' })).toBe('FR Full : SRT');
  });
  it('peu de répliques (sub_forced_hint) → Forced', () => {
    expect(inferSubLabel({ language: 'fre', codec_id: 'S_TEXT/UTF8', sub_forced_hint: true })).toBe('FR Forced : SRT');
  });
  it('« Canadian » → FR VFQ', () => {
    expect(inferSubLabel({ language: 'fre', codec_id: 'S_TEXT/UTF8', track_name: 'French Canadian' })).toBe('FR VFQ Full : SRT');
  });
  it('PGS détecté via le codec', () => {
    expect(inferSubLabel({ language: 'fre', codec_id: 'S_HDMV/PGS' })).toBe('FR Full : PGS');
  });
  it('SDH anglais', () => {
    expect(inferSubLabel({ language: 'eng', codec_id: 'S_TEXT/UTF8', track_name: 'English SDH' })).toBe('ENG SDH : SRT');
  });
  it("une autre langue n'est jamais confondue avec ENG", () => {
    expect(inferSubLabel({ language: 'hun', codec_id: 'S_TEXT/UTF8' })).toBe('HUN Full : SRT');
  });
});

describe('langFlag (MULTi / DUAL)', () => {
  it('VFi + VO avec sous-titre FR → MULTi.VFi', () => {
    expect(langFlag(['FR VFi : AC3 5.1', 'ENG VO : AC3 5.1'], true)).toBe('MULTi.VFi');
  });
  it('VFi + VO sans sous-titre FR → DUAL.VFi', () => {
    expect(langFlag(['FR VFi : AC3 5.1', 'ENG VO : AC3 5.1'], false)).toBe('DUAL.VFi');
  });
  it('VFF + VFQ → VF2', () => {
    expect(langFlag(['FR VFF : AC3 5.1', 'FR VFQ : AC3 5.1', 'ENG VO : AC3 5.1'], true)).toBe('MULTi.VF2');
  });
  it('film français : VOF seul ou avec VO', () => {
    expect(langFlag(['FR VOF : AC3 5.1'], true)).toBe('FRENCH.VOF');
    expect(langFlag(['FR VOF : AC3 5.1', 'ENG VO : AC3 5.1'], true)).toBe('MULTi.FRENCH');
  });
  it('VO seule', () => {
    expect(langFlag(['ENG VO : EAC3 5.1'], false)).toBe('VO');
  });
});

describe('subLabelPriority (ordre des sous-titres)', () => {
  it('FR Forced < FR Full < FR VFQ < ENG', () => {
    const labels = ['ENG Full : SRT', 'FR VFQ Full : SRT', 'FR Full : SRT', 'FR Forced : SRT'];
    expect([...labels].sort((a, b) => subLabelPriority(a) - subLabelPriority(b)))
      .toEqual(['FR Forced : SRT', 'FR Full : SRT', 'FR VFQ Full : SRT', 'ENG Full : SRT']);
  });
  it('SRT avant PGS à égalité', () => {
    expect(subLabelPriority('FR Full : SRT')).toBeLessThan(subLabelPriority('FR Full : PGS'));
  });
});

describe('looksLikeAD / mapLangCode / dotify', () => {
  it('AD seulement avec marqueur explicite', () => {
    expect(looksLikeAD({ track_name: 'Audiodescription' })).toBe(true);
    expect(looksLikeAD({ flag_visual_impaired: true })).toBe(true);
    expect(looksLikeAD({ track_name: 'French' })).toBe(false);
  });
  it('codes langue Matroska', () => {
    expect(mapLangCode('FR VFQ : AC3 5.1')).toBe('fr-ca');
    expect(mapLangCode('FR VFF : AC3 5.1')).toBe('fre');
    expect(mapLangCode('ENG Full : SRT')).toBe('eng');
  });
  it('titre en points, & → And (chaque mot capitalisé)', () => {
    expect(dotify('Law & Order: Toronto')).toBe('Law.And.Order.Toronto');
    expect(dotify('jurassic world fallen kingdom')).toBe('Jurassic.World.Fallen.Kingdom');
  });
});

import { normalizeTitle, yearFromFilename, findUnambiguousTmdbMatch } from './rules.js';

describe('validation TMDB automatique', () => {
  it('normalise accents, & et ponctuation', () => {
    expect(normalizeTitle('Law & Order Toronto: Criminal Intent')).toBe('law and order toronto criminal intent');
    expect(normalizeTitle('Amélie')).toBe('amelie');
  });
  it("lit l'année du fichier sans confondre avec 1080p", () => {
    expect(yearFromFilename('Fortress.2012.1080p.BluRay.mkv')).toBe('2012');
    expect(yearFromFilename('Show.S01E01.1080p.WEB.mkv')).toBe('');
  });
  it('un seul titre exact → validé, même si ce n\'est pas le 1er résultat', () => {
    const r = [{ titre_vo: 'The Terminal List: Dark Wolf' }, { titre_vo: 'The Terminal List' }];
    expect(findUnambiguousTmdbMatch('The Terminal List', '', r)).toBe(1);
  });
  it('& dans TMDB vs « and » dans le fichier', () => {
    const r = [{ titre_vo: 'Law & Order Toronto: Criminal Intent', titre_fr: 'Toronto: Section Criminelle' }];
    expect(findUnambiguousTmdbMatch('Law and Order Toronto Criminal Intent', '', r)).toBe(0);
  });
  it("deux remakes du même titre sans année → confirmation demandée", () => {
    const r = [{ titre_vo: 'The Office', annee_fr: '2005' }, { titre_vo: 'The Office', annee_fr: '2001' }];
    expect(findUnambiguousTmdbMatch('The Office', '', r)).toBe(-1);
  });
  it("l'année du fichier départage", () => {
    const r = [{ titre_vo: 'Dune', annee_fr: '1984' }, { titre_vo: 'Dune', annee_fr: '2021' }];
    expect(findUnambiguousTmdbMatch('Dune', '2021', r)).toBe(1);
  });
  it('titre seulement approchant → confirmation demandée', () => {
    expect(findUnambiguousTmdbMatch('Boston Blue', '', [{ titre_vo: 'Boston Legal' }])).toBe(-1);
  });
});

import { pairSeasonFiles } from './rules.js';

describe('pairSeasonFiles (mode Saison)', () => {
  const d = '/Saison/';
  it('PSA : associe chaque PSA à son SUPPLY par épisode, triés', () => {
    const r = pairSeasonFiles([
      d + 'Show.S01E02.1080p.WEB.H264-SUPPLY.mkv',
      d + 'Show.S01E01.1080p.WEBRip.x265-PSA.mkv',
      d + 'Show.S01E01.MULTi.1080p.WEB.H264-SUPPLY.mkv',
      d + 'Show.S01E02.1080p.WEBRip.x265-PSA.mkv',
    ], 'psa');
    expect(r.episodes.map((e) => e.episode)).toEqual(['S01E01', 'S01E02']);
    expect(r.episodes[0].source).toContain('S01E01.1080p.WEBRip.x265-PSA');
    expect(r.episodes[0].supply).toContain('S01E01.MULTi');
    expect(r.episodes[0].issues).toEqual([]);
  });
  it('PSA : signale un SUPPLY manquant', () => {
    const r = pairSeasonFiles([d + 'Show.S01E03.1080p.WEBRip.x265-PSA.mkv'], 'psa');
    expect(r.episodes[0].issues).toContain('SUPPLY manquant');
  });
  it('LiHDL : un fichier par épisode, fichiers sans SxxExx ignorés', () => {
    const r = pairSeasonFiles([d + 'Show.S02E01.mkv', d + 'Show.S02E02.mkv', d + 'Bonus.Making.Of.mkv'], 'lihdl');
    expect(r.episodes).toHaveLength(2);
    expect(r.ignored).toEqual([d + 'Bonus.Making.Of.mkv']);
  });
  it('signale les doublons', () => {
    const r = pairSeasonFiles([d + 'Show.S01E01.A.mkv', d + 'Show.S01E01.B.mkv'], 'lihdl');
    expect(r.episodes[0].issues[0]).toMatch(/plusieurs fichiers/);
  });
});

import { carryManualFilename } from './rules.js';

describe('carryManualFilename (nom modifié à la main, épisode suivant)', () => {
  it('garde la retouche et met à jour le SxxExx', () => {
    expect(carryManualFilename('American.Horror.Story.S13E04.VOSTFR.1080p.WEB.EAC3.5.1.H264-LiHDL.mkv', 'S13E08'))
      .toBe('American.Horror.Story.S13E08.VOSTFR.1080p.WEB.EAC3.5.1.H264-LiHDL.mkv');
  });
  it('sans SxxExx dans le nom → retour au nom auto', () => {
    expect(carryManualFilename('Film.2024.VOSTFR.1080p.mkv', 'S01E02')).toBe('');
  });
  it('nouvel épisode inconnu → retour au nom auto', () => {
    expect(carryManualFilename('Show.S01E01.VOSTFR.mkv', '')).toBe('');
  });
});
