from math import log
import spacy
import string

class Vocabulary(object):
    def __init__(self):
        self.maxword = 30
        self.parser = spacy.load('en_core_web_md')
        self.vocab = self.parser.vocab
        self.low_prob = self.vocab['sjflsjl'].prob
        self.new_words = set()

    def infer_spaces(self, s):
        # Find the best match for the n first characters, assuming prob has
        # been built for the n-1 first characters.
        # Returns a pair (match_prob, match_length).
        # prob[0] is set to 0. prob[n] stores the best probability
        # for the string s[0:n]
        def best_match(n):
            max_prob = -1e99
            best_pos = 0
            for i in range(max(0, n - self.maxword), n):
                new_word = s[i:n]
                new_prob = self.vocab[new_word].prob
                if new_prob == self.low_prob and new_word in self.new_words:
                    new_prob = -3.0 # just assume 1000 words equal opportunity

                total_prob = prob[i] + new_prob
                if total_prob > max_prob:
                    max_prob = total_prob
                    best_pos = i
            return max_prob, best_pos

        # Build the prob array. We start the first entry as 0 to avoid checking for boundary condition.
        # n passed to best_match is the string len we currently evaluate
        prob = [0]
        for n in range(1, len(s)+1):
            p, i = best_match(n)
            prob.append(p)

        # Backtrack to recover the max-probability string.
        out = []
        i = len(s)
        while i > 0:
            c, k = best_match(i)
            assert c == prob[i]
            out.insert(0, s[k:i])
            i = k

        return out

    # normalize a word (or phrase). For any new word not in our dictionary we add them.
    def normalize(self, new_word):
        norm_word = ''
        # we always treat the new words' cost as zero, since these are the words that exist in our swagger.yaml.
        individual_words = self.infer_spaces(new_word.lower())
        for w in individual_words:
            if not w.isalpha():
                continue

            if self.vocab[w].prob <= self.low_prob:
                # it's a new word, add to our side dict
                self.new_words.add(w)

            if w == 'id':
                norm_word += 'id '
            else:
                tokens = self.parser(w)
                for t in tokens:
                    norm_word += t.lemma_ + ' '
        return norm_word[:-1]
