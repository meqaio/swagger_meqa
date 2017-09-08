from math import log

class Vocabulary(object):
    def __init__(self):
        self.words = open("words.txt").read().split()
        self.wordcost = dict((k, log((i + 1) * log(len(self.words)))) for i, k in enumerate(self.words))
        self.maxword = max(len(x) for x in self.words)

    def infer_spaces(self, s):
        """Uses dynamic programming to infer the location of spaces in a string
        without spaces."""

        # Find the best match for the i first characters, assuming cost has
        # been built for the i-1 first characters.
        # Returns a pair (match_cost, match_length).
        def best_match(i):
            candidates = enumerate(reversed(cost[max(0, i - self.maxword):i]))
            return min((c + self.wordcost.get(s[i - k - 1:i], 1e99), k + 1) for k, c in candidates)

        # Build the cost array.
        cost = [0]
        for i in range(1, len(s) + 1):
            c, k = best_match(i)
            cost.append(c)

        # Backtrack to recover the minimal-cost string.
        out = []
        i = len(s)
        while i > 0:
            c, k = best_match(i)
            assert c == cost[i]
            out.insert(0, s[i - k:i])
            i -= k

        return out

    # add a new word, return the properly broken down word
    def add_word(self, new_word):
        # we always treat the new words' cost as zero, since these are the words that exist in our swagger.yaml.
        individual_words = self.infer_spaces(new_word.lower())
        for w in individual_words:
            self.wordcost[w] = 0
            self.maxword = max(self.maxword, len(w))

        return " ".join(individual_words)