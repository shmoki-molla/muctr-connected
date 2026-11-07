export type FileKind = 'pdf' | 'image' | 'docx' | 'pptx' | 'xlsx' | 'zip';

export type DriveFolder = {
  id: string;
  name: string;
  count: number;
};

export type DriveFile = {
  id: string;
  name: string;
  owner: string;
  modified: string;
  size: string;
  kind: FileKind;
  folderId: string;
};

export const folders: DriveFolder[] = [
  { id: 'achievements', name: 'Достижения', count: 12 },
  { id: 'projects', name: 'Проекты', count: 8 },
  { id: 'coursework', name: 'Курсовые работы', count: 15 },
  { id: 'practice', name: 'Практики', count: 6 },
  { id: 'certificates', name: 'Сертификаты', count: 9 },
  { id: 'olympiads', name: 'Олимпиады', count: 5 },
];

export const files: DriveFile[] = [
  {
    id: 'f-1',
    folderId: 'achievements',
    name: 'Диплом за олимпиаду.pdf',
    owner: 'Я',
    modified: '12 мая 2024',
    size: '2,4 МБ',
    kind: 'pdf',
  },
  {
    id: 'f-2',
    folderId: 'achievements',
    name: 'Сертификат конференции.jpg',
    owner: 'Я',
    modified: '11 мая 2024',
    size: '1,1 МБ',
    kind: 'image',
  },
  {
    id: 'f-3',
    folderId: 'achievements',
    name: 'Курсовая работа.docx',
    owner: 'Я',
    modified: '10 мая 2024',
    size: '3,7 МБ',
    kind: 'docx',
  },
  {
    id: 'f-4',
    folderId: 'achievements',
    name: 'Презентация проекта.pptx',
    owner: 'Я',
    modified: '8 мая 2024',
    size: '5,2 МБ',
    kind: 'pptx',
  },
  {
    id: 'f-5',
    folderId: 'achievements',
    name: 'Грамота за участие.pdf',
    owner: 'Я',
    modified: '7 мая 2024',
    size: '1,8 МБ',
    kind: 'pdf',
  },
  {
    id: 'f-6',
    folderId: 'achievements',
    name: 'Отчет по практике.pdf',
    owner: 'Я',
    modified: '5 мая 2024',
    size: '2,9 МБ',
    kind: 'pdf',
  },
  {
    id: 'f-7',
    folderId: 'projects',
    name: 'Информационная платформа.pdf',
    owner: 'Я',
    modified: '3 мая 2024',
    size: '4,0 МБ',
    kind: 'pdf',
  },
  {
    id: 'f-8',
    folderId: 'projects',
    name: 'Презентация.pptx',
    owner: 'Я',
    modified: '1 мая 2024',
    size: '6,5 МБ',
    kind: 'pptx',
  },
  {
    id: 'f-9',
    folderId: 'projects',
    name: 'Список участников.xlsx',
    owner: 'Я',
    modified: '29 апр 2024',
    size: '890 КБ',
    kind: 'xlsx',
  },
  {
    id: 'f-10',
    folderId: 'coursework',
    name: 'Технологии программирования.docx',
    owner: 'Я',
    modified: '27 апр 2024',
    size: '2,1 МБ',
    kind: 'docx',
  },
  {
    id: 'f-11',
    folderId: 'coursework',
    name: 'Расчеты и приложения.zip',
    owner: 'Я',
    modified: '22 апр 2024',
    size: '12,4 МБ',
    kind: 'zip',
  },
  {
    id: 'f-12',
    folderId: 'practice',
    name: 'Дневник практики.pdf',
    owner: 'Я',
    modified: '18 апр 2024',
    size: '2,0 МБ',
    kind: 'pdf',
  },
  {
    id: 'f-13',
    folderId: 'certificates',
    name: 'Сертификат Python.pdf',
    owner: 'Я',
    modified: '13 апр 2024',
    size: '1,6 МБ',
    kind: 'pdf',
  },
  {
    id: 'f-14',
    folderId: 'olympiads',
    name: 'Заявка на олимпиаду.docx',
    owner: 'Я',
    modified: '9 апр 2024',
    size: '740 КБ',
    kind: 'docx',
  },
];
