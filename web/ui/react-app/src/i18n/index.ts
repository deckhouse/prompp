import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import { ru } from './locales/ru';

export const LOCAL_STORAGE_LANGUAGE_KEY = 'language';
export const DEFAULT_LANGUAGE = 'en';
const defaultLng = localStorage.getItem(LOCAL_STORAGE_LANGUAGE_KEY) || DEFAULT_LANGUAGE;

i18n.use(initReactI18next).init({
  resources: {
    ru,
  },
  lng: defaultLng,
  debug: true,
  interpolation: {
    escapeValue: false,
  },
});

export default i18n;
