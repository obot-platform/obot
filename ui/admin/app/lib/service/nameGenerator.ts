import { faker } from "@faker-js/faker";

const uppercaseFirst = (word: string) =>
    word.charAt(0).toUpperCase() + word.slice(1);

export function generateRandomName(): string {
    return [faker.word.adjective(), faker.word.noun()]
        .map(uppercaseFirst)
        .join(" ");
}
