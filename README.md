# Wypożyczalnia pojazdów

Program do prowadzenia wypożyczalni samochodów, quadów i innych pojazdów:
rezerwacje, kalendarz dostępności, klienci, cennik, wyposażenie dodatkowe,
limit kilometrów, kaucje, wydanie i zwrot pojazdu oraz wydruk umowy i protokołu.

## Instalacja (dla użytkownika)

1. Pobierz plik **`wypozyczalnia.exe`** z zakładki
   [Releases](https://github.com/ppolakiewicz/vehicle-rent-app/releases/latest).
2. Przenieś go np. na Pulpit i uruchom dwuklikiem. Program otworzy się w osobnym oknie.
3. Przy pierwszym uruchomieniu Windows może pokazać okno
   „System Windows ochronił ten komputer” – kliknij **Więcej informacji → Uruchom mimo to**.
   (Program nie ma płatnego podpisu cyfrowego.)

Nie trzeba niczego instalować. Program działa bez internetu.

**Aktualizacja:** pobierz nowy `wypozyczalnia.exe` i zastąp nim stary. Dane zostają.

**Dane** są w folderze `Dokumenty\Wypożyczalnia`:
- `wypozyczalnia.db` – baza danych,
- `kopie\` – kopie zapasowe, robione automatycznie przy każdym uruchomieniu (30 ostatnich),
- `dziennik.log` – dziennik programu (przydatny przy zgłaszaniu błędów).

Warto co jakiś czas skopiować folder `kopie` na pendrive lub do chmury.
Aby odtworzyć dane z kopii: zamknij program, zastąp `wypozyczalnia.db` wybranym plikiem z `kopie\`
(zmieniając mu nazwę na `wypozyczalnia.db`) i uruchom program.

## Jak działa

- Jeden plik `.exe` napisany w Go, z wbudowaną bazą SQLite (bez dodatkowych bibliotek).
- Interfejs to strony HTML (z HTMX) serwowane lokalnie na `127.0.0.1:47800`
  i pokazywane w oknie Microsoft Edge w trybie aplikacji (bez paska adresu).
  Edge jest w każdym Windows 10/11; gdy go brak, użyty zostanie Chrome lub domyślna przeglądarka.
- Zamknięcie okna kończy program. Ponowne uruchomienie, gdy program działa, otwiera kolejne okno.

### Zasady rozliczeń

- Każde wypożyczenie ma własne ceny (netto): za dobę/godzinę, za km ponad limit, kaucję
  i cenę każdej pozycji wyposażenia. Cennik jest tylko szablonem: jego ceny wpisują się
  do formularza (z domyślnego cennika pojazdu lub wybranego ręcznie) i można je zmienić.
  Zmiana cennika nie zmienia zapisanych wypożyczeń.
- VAT (domyślnie 23%) doliczany jest do sumy netto.
- Rozliczenie dobowe: każda rozpoczęta doba, z tolerancją (domyślnie 60 min) na ostatnią dobę.
- Rozliczenie godzinowe: każda rozpoczęta godzina.
- Wyposażenie dodatkowe: opłata jednorazowa za sztukę (cena domyślna z listy wyposażenia,
  do zmiany przy wypożyczeniu); liczba sztuk jest ograniczona.
- Limit km (opcjonalny) na cały najem; za każdy km ponad limit stawka z wypożyczenia.
- Późny zwrot: liczony jest faktyczny czas zwrotu (wcześniejszy zwrot – planowany).
- Kaucja osobno, bez VAT; przy zwrocie można zatrzymać jej część.

## Rozwój

```bash
go test ./...
go run ./cmd/wypozyczalnia -data ./dane-dev -no-window   # http://127.0.0.1:47800
GOOS=windows GOARCH=amd64 go build -ldflags "-H windowsgui" -o wypozyczalnia.exe ./cmd/wypozyczalnia
```

Struktura:

| Katalog | Zawartość |
|---|---|
| `cmd/wypozyczalnia` | start programu, okno, zamykanie |
| `internal/core` | kwoty, daty, wycena (bez bazy, z testami) |
| `internal/db` | SQLite, migracje (`migrations/*.sql`), kopie zapasowe |
| `internal/store` | zapytania: pojazdy, klienci, wypożyczenia, dostępność |
| `internal/web` | serwer HTTP, szablony, CSS/JS |
| `internal/launcher` | Windows: folder Dokumenty, Edge, okna komunikatów |

Zmiana schematu bazy: dodaj plik `internal/db/migrations/000N_opis.sql` – zostanie
zastosowany automatycznie przy starcie (stan zapisany w `PRAGMA user_version`).

### Wydanie nowej wersji

Repozytorium główne jest na Forgejo, z lustrem na GitHub. Plik `.exe` buduje
GitHub Actions (`.github/workflows/build.yml`):

```bash
git tag v0.2.0 && git push origin v0.2.0
```

Po kilku minutach plik pojawi się w GitHub Releases. Forgejo uruchamia tylko testy
(`.forgejo/workflows/test.yml`).
